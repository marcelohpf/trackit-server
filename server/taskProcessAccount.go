//   Copyright 2017 MSolution.IO
//
//   Licensed under the Apache License, Version 2.0 (the "License");
//   you may not use this file except in compliance with the License.
//   You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
//   Unless required by applicable law or agreed to in writing, software
//   distributed under the License is distributed on an "AS IS" BASIS,
//   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//   See the License for the specific language governing permissions and
//   limitations under the License.

package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"strconv"
	"time"

	"github.com/trackit/jsonlog"

	"github.com/trackit/trackit-server/aws"
	"github.com/trackit/trackit-server/aws/usageReports/ec2"
	"github.com/trackit/trackit-server/aws/usageReports/elasticache"
	"github.com/trackit/trackit-server/aws/usageReports/es"
	"github.com/trackit/trackit-server/aws/usageReports/history"
	"github.com/trackit/trackit-server/aws/usageReports/lambda"
	"github.com/trackit/trackit-server/aws/usageReports/rds"
	"github.com/trackit/trackit-server/aws/usageReports/riEc2"
	"github.com/trackit/trackit-server/aws/usageReports/riRdS"
	"github.com/trackit/trackit-server/db"
	"github.com/trackit/trackit-server/onDemandToRI/ec2"
)

// taskProcessAccount processes an AwsAccount to retrieve data from the AWS api.
func taskProcessAccount(ctx context.Context) error {
	args := flag.Args()
	logger := jsonlog.LoggerFromContextOrDefault(ctx)
	logger.Debug("Running task 'process-account'.", map[string]interface{}{
		"args": args,
	})
	if len(args) != 1 {
		return errors.New("taskProcessAccount requires an integer argument")
	} else if aaId, err := strconv.Atoi(args[0]); err != nil {
		return err
	} else {
		return ingestDataForAccount(ctx, aaId)
	}
}

// ingestDataForAccount ingests the AWS api data for an AwsAccount.
func ingestDataForAccount(ctx context.Context, aaId int) (err error) {
	var tx *sql.Tx
	var aa aws.AwsAccount
	var updateId int64
	logger := jsonlog.LoggerFromContextOrDefault(ctx)
	defer func() {
		if tx != nil {
			if err != nil {
				tx.Rollback()
			} else {
				tx.Commit()
			}
		}
	}()
	if tx, err = db.Db.BeginTx(ctx, nil); err != nil {
	} else if aa, err = aws.GetAwsAccountWithId(aaId, tx); err != nil {
	} else if updateId, err = registerAccountProcessing(db.Db, aa); err != nil {
	} else {
		ec2Err := processAccountEC2(ctx, aa)
		rdsErr := processAccountRDS(ctx, aa)
		esErr := processAccountES(ctx, aa)
		elastiCacheErr := processAccountElastiCache(ctx, aa)
		lambdaErr := processAccountLambda(ctx, aa)
		riEc2Err := riEc2.FetchDailyReservationsStats(ctx, aa)
		riRdsErr := riRdS.FetchDailyInstancesStats(ctx, aa)
		odToRiEc2Err := onDemandToRiEc2.RunOnDemandToRiEc2(ctx, aa)
		historyCreated, historyErr := processAccountHistory(ctx, aa)
		updateAccountProcessingCompletion(ctx, aaId, db.Db, updateId, nil, rdsErr, ec2Err, esErr, elastiCacheErr, lambdaErr, riEc2Err, riRdsErr, odToRiEc2Err, historyErr, historyCreated)
	}
	if err != nil {
		updateAccountProcessingCompletion(ctx, aaId, db.Db, updateId, err, nil, nil, nil, nil, nil, nil, nil, nil, nil, false)
		logger.Error("Failed to process account data.", map[string]interface{}{
			"awsAccountId": aaId,
			"error":        err.Error(),
		})
	}
	return
}

func registerAccountProcessing(db *sql.DB, aa aws.AwsAccount) (int64, error) {
	const sqlstr = `INSERT INTO aws_account_update_job(
		aws_account_id,
		worker_id
	) VALUES (?, ?)`
	res, err := db.Exec(sqlstr, aa.Id, backendId)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func updateAccountProcessingCompletion(ctx context.Context, aaId int, db *sql.DB, updateId int64, jobErr, rdsErr, ec2Err, esErr, elastiCacheErr, lambdaErr, riEc2Err, riRdsErr, odToRiEc2Err, historyErr error, historyCreated bool) {
	updateNextUpdateAccount(db, aaId)
	rErr := registerAccountProcessingCompletion(db, updateId, jobErr, rdsErr, ec2Err, esErr, elastiCacheErr, lambdaErr, riEc2Err, riRdsErr, odToRiEc2Err, historyErr, historyCreated)
	if rErr != nil {
		logger := jsonlog.LoggerFromContextOrDefault(ctx)
		logger.Error("Failed to register account processing completion.", map[string]interface{}{
			"awsAccountId": aaId,
			"error":        rErr.Error(),
			"updateId":     updateId,
		})
	}
}

func errToStr(err error) string {
	if err != nil {
		return err.Error()
	}
	return ""
}

func updateNextUpdateAccount(db *sql.DB, aaId int) error {
	const sqlstr = `UPDATE aws_account SET
		next_update=?
	WHERE id=?`
	_, err := db.Exec(sqlstr, time.Now().AddDate(0, 0, 1), aaId)
	return err
}

func registerAccountProcessingCompletion(db *sql.DB, updateId int64, jobErr, rdsErr, ec2Err, esErr, elastiCacheErr, lambdaErr, riEc2Err, riRdsErr, odToRiEc2Err, historyErr error, historyCreated bool) error {
	const sqlstr = `UPDATE aws_account_update_job SET
		completed=?,
		jobError=?,
		rdsError=?,
		ec2Error=?,
		esError=?,
		elastiCacheError=?,
		lambdaError=?,
		riEc2Error=?,
		riRdsError=?,
		odToRiEc2Error=?,
		historyError=?,
		monthly_reports_generated=?
	WHERE id=?`
	_, err := db.Exec(sqlstr, time.Now(), errToStr(jobErr), errToStr(rdsErr), errToStr(ec2Err), errToStr(esErr), errToStr(elastiCacheErr), errToStr(lambdaErr), errToStr(riEc2Err), errToStr(riRdsErr), errToStr(odToRiEc2Err), errToStr(historyErr), historyCreated, updateId)
	return err
}

// processAccountRDS processes all the RDS data for an AwsAccount
func processAccountRDS(ctx context.Context, aa aws.AwsAccount) error {
	err := rds.FetchDailyInstancesStats(ctx, aa)
	if err != nil {
		logger := jsonlog.LoggerFromContextOrDefault(ctx)
		logger.Error("Failed to ingest RDS data.", map[string]interface{}{
			"awsAccountId": aa.Id,
			"error":        err.Error(),
		})
	}
	return err
}

// processAccountEC2 processes all the EC2 data for an AwsAccount
func processAccountEC2(ctx context.Context, aa aws.AwsAccount) error {
	err := ec2.FetchDailyInstancesStats(ctx, aa)
	if err != nil {
		logger := jsonlog.LoggerFromContextOrDefault(ctx)
		logger.Error("Failed to ingest EC2 data.", map[string]interface{}{
			"awsAccountId": aa.Id,
			"error":        err.Error(),
		})
	}
	return err
}

// processAccountES processes all the ES data for an AwsAccount
func processAccountES(ctx context.Context, aa aws.AwsAccount) error {
	err := es.FetchDomainsStats(ctx, aa)
	if err != nil {
		logger := jsonlog.LoggerFromContextOrDefault(ctx)
		logger.Error("Failed to ingest ES data", map[string]interface{}{
			"awsAccountId": aa.Id,
			"error":        err.Error(),
		})
	}
	return err
}

// processAccountElastiCache processes all the ElastiCache data for an AwsAccount
func processAccountElastiCache(ctx context.Context, aa aws.AwsAccount) error {
	err := elasticache.FetchDailyInstancesStats(ctx, aa)
	if err != nil {
		logger := jsonlog.LoggerFromContextOrDefault(ctx)
		logger.Error("Failed to ingest ElastiCache data", map[string]interface{}{
			"awsAccountId": aa.Id,
			"error":        err.Error(),
		})
	}
	return err
}

// processAccountLambda processes all the Lambda data for an AwsAccount
func processAccountLambda(ctx context.Context, aa aws.AwsAccount) error {
	err := lambda.FetchDailyFunctionsStats(ctx, aa)
	if err != nil {
		logger := jsonlog.LoggerFromContextOrDefault(ctx)
		logger.Error("Failed to ingest Lambda data", map[string]interface{}{
			"awsAccountId": aa.Id,
			"error":        err.Error(),
		})
	}
	return err
}

// processAccountHistory processes EC2, RDS, ES, ElastiCache, Lambda and EC2 Coverage data with billing data for an AwsAccount
func processAccountHistory(ctx context.Context, aa aws.AwsAccount) (bool, error) {
	status, err := history.FetchHistoryInfos(ctx, aa)
	if err != nil && err != history.ErrBillingDataIncomplete {
		logger := jsonlog.LoggerFromContextOrDefault(ctx)
		logger.Error("Failed to ingest History data.", map[string]interface{}{
			"awsAccountId": aa.Id,
			"error":        err.Error(),
		})
	}
	return status, err
}
