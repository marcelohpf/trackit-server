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

	"github.com/trackit/jsonlog"
	"github.com/trackit/trackit-server/aws"
	//"github.com/trackit/trackit-server/aws/ri"
	"github.com/trackit/trackit-server/aws/usageReports/history"
	"github.com/trackit/trackit-server/config"
	"github.com/trackit/trackit-server/db"
)

// taskProcessAccount processes an AwsAccount to retrieve data from the AWS api.
// Do not pass the Master Trackit Account to run for a specific Aws Account
func taskMonthHistoryAccount(ctx context.Context) error {
	args := flag.Args()
	logger := jsonlog.LoggerFromContextOrDefault(ctx)
	logger.Debug("Running task 'process-month-history'.", map[string]interface{}{
		"args":         args,
		"master-email": config.MasterEmail,
	})
	if config.MasterEmail != "" {
		return ingestMonthHistoryDataForAccount(ctx, 0)
	} else if len(args) != 1 {
		return errors.New("taskMonthHistoryAccount requires an integer argument or a Master Trackit Account (master-email).")
	} else if aaId, err := strconv.Atoi(args[0]); err != nil {
		return err
	} else {
		return ingestMonthHistoryDataForAccount(ctx, aaId)
	}
	// return ingestDataForAccount(ctx, 1)
}

// ingestHistoryDataForAccount ingests the AWS api data for an AwsAccount.
func ingestMonthHistoryDataForAccount(ctx context.Context, aaId int) (err error) {
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
	} else if aa, err = getAwsAccount(ctx, aaId, tx); err != nil {
	} else if updateId, err = registerAccountProcessing(db.Db, aa); err != nil {
	} else {
		historyErr := processAccountHistory(ctx, aa)
		updateAccountProcessingCompletion(ctx, aaId, db.Db, updateId, nil, nil, nil, historyErr, nil, nil)
	}
	if err != nil {
		updateAccountProcessingCompletion(ctx, aaId, db.Db, updateId, err, nil, nil, nil, nil, nil)
		logger.Error("Failed to process account data.", map[string]interface{}{
			"awsAccountId": aaId,
			"error":        err.Error(),
		})
	}
	return
}

// processAccountHistory processes EC2 and RDS data with billing data for an AwsAccount
func processAccountHistory(ctx context.Context, aa aws.AwsAccount) error {
	err := history.FetchHistoryInfos(ctx, aa)
	if err != nil {
		logger := jsonlog.LoggerFromContextOrDefault(ctx)
		logger.Error("Failed to ingest History data.", map[string]interface{}{
			"awsAccountId": aa.Id,
			"error":        err.Error(),
		})
	}
	return err
}
