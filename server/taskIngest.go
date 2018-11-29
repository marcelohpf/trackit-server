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
	"math/rand"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go/aws/awserr"

	"github.com/trackit/jsonlog"

	"github.com/trackit/trackit-server/aws"
	"github.com/trackit/trackit-server/aws/s3"
	"github.com/trackit/trackit-server/config"
	"github.com/trackit/trackit-server/db"
	"github.com/trackit/trackit-server/users"
)

// taskIngest ingests billing data for a given BillRepository and AwsAccount.
func taskIngest(ctx context.Context) error {
	args := flag.Args()
	logger := jsonlog.LoggerFromContextOrDefault(ctx)
	logger.Debug("Running task 'ingest'.", map[string]interface{}{
		"args":         args,
		"master-email": config.MasterEmail,
	})
	if config.MasterEmail != "" {
		return ingestBillingDataForBillRepository(ctx, 0, 0)
	} else if len(args) != 2 {
		return errors.New("taskIngest requires two integer arguments or a Master Trackit Account (master-email)")
	} else if aa, err := strconv.Atoi(args[0]); err != nil {
		return err
	} else if br, err := strconv.Atoi(args[1]); err != nil {
		return err
	} else {
		return ingestBillingDataForBillRepository(ctx, aa, br)
	}
	//return ingestBillingDataForBillRepository(ctx, 1, 1)
}

// ingestBillingDataForBillRepository ingests the billing data for a
// BillRepository.
func ingestBillingDataForBillRepository(ctx context.Context, aaId, brId int) (err error) {
	var tx *sql.Tx
	var aa aws.AwsAccount
	var br s3.BillRepository
	var updateId int64
	var latestManifest time.Time
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
	} else if br, err = getBillRespository(ctx, aaId, brId, tx); err != nil {
	} else if updateId, err = registerUpdate(db.Db, br); err != nil {
	} else if latestManifest, err = s3.UpdateReport(ctx, aa, br); err != nil {
		if billError, castok := err.(awserr.Error); castok {
			br.Error = billError.Message()
			s3.UpdateBillRepositoryWithoutContext(br, db.Db)
		}
	} else {
		br.Error = ""
		err = updateBillRepositoryForNextUpdate(ctx, tx, br, latestManifest)
	}
	if err != nil {
		logger.Error("Failed to ingest billing data.", map[string]interface{}{
			"awsAccountId":     aaId,
			"billRepositoryId": brId,
			"error":            err.Error(),
		})
	}
	updateCompletion(ctx, aaId, brId, db.Db, updateId, err)
	return
}

// getBillRepository search for the bill repository using the AwsAccount and
// Bill Repository ID or use the Master Trackit Account
func getBillRespository(ctx context.Context, aaId, brId int, tx *sql.Tx) (s3.BillRepository, error) {
	var user users.User
	var aas []aws.AwsAccount
	var aa aws.AwsAccount
	var brs []s3.BillRepository
	var br s3.BillRepository
	var err error
	if aaId == 0 || brId == 0 {
		if user, err = users.GetUserWithEmail(ctx, tx, config.MasterEmail); err != nil {
		} else if aas, err = aws.GetAwsAccountsFromUser(user, tx); err != nil || len(aas) == 0 {
		} else if brs, err = s3.GetBillRepositoriesForAwsAccount(aas[0], tx); err != nil || len(brs) == 0 {
		} else {
			return brs[0], nil
		}
	} else if aa, err = aws.GetAwsAccountWithId(aaId, tx); err != nil {
	} else if br, err = s3.GetBillRepositoryForAwsAccountById(aa, brId, tx); err != nil {
	} else {
		return br, nil
	}
	return br, err
}

func registerUpdate(db *sql.DB, br s3.BillRepository) (int64, error) {
	const sqlstr = `INSERT INTO aws_bill_update_job(
		aws_bill_repository_id,
		worker_id,
		error
	) VALUES (?, ?, "")`
	res, err := db.Exec(sqlstr, br.Id, backendId)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func updateCompletion(ctx context.Context, aaId, brId int, db *sql.DB, updateId int64, err error) {
	rErr := registerUpdateCompletion(db, updateId, err)
	if rErr != nil {
		logger := jsonlog.LoggerFromContextOrDefault(ctx)
		logger.Error("Failed to register ingestion completion.", map[string]interface{}{
			"awsAccountId":     aaId,
			"billRepositoryId": brId,
			"error":            rErr.Error(),
			"updateId":         updateId,
		})
	}
}

func registerUpdateCompletion(db *sql.DB, updateId int64, err error) error {
	const sqlstr = `UPDATE aws_bill_update_job SET
		completed=?,
		error=?
	WHERE id=?`
	var errorValue string
	var now = time.Now()
	if err != nil {
		errorValue = err.Error()
	}
	_, err = db.Exec(sqlstr, now, errorValue, updateId)
	return err
}

const (
	UpdateIntervalMinutes = 6 * 60
	UpdateIntervalWindow  = 2 * 60
)

// updateBillRepositoryForNextUpdate plans the next update for a
// BillRepository.
func updateBillRepositoryForNextUpdate(ctx context.Context, tx *sql.Tx, br s3.BillRepository, latestManifest time.Time) error {
	if latestManifest.After(br.LastImportedManifest) {
		br.LastImportedManifest = latestManifest
	}
	updateDeltaMinutes := time.Duration(UpdateIntervalMinutes-UpdateIntervalWindow/2+rand.Int63n(UpdateIntervalWindow)) * time.Minute
	br.NextUpdate = time.Now().Add(updateDeltaMinutes)
	return s3.UpdateBillRepository(br, tx)
}
