package main

import (
	"context"
	"database/sql"
	"errors"
	"github.com/trackit/trackit-server/mail"
	"math/rand"
	"time"

	"github.com/trackit/jsonlog"
	"github.com/trackit/trackit-server/aws"
	"github.com/trackit/trackit-server/aws/s3"
	"github.com/trackit/trackit-server/config"
	"github.com/trackit/trackit-server/db"
	"github.com/trackit/trackit-server/users"
)

// taskSetupMasterAccount create and validate the creation for a Master Trackit Account
func taskSetupMasterAccount(ctx context.Context) (err error) {
	logger := jsonlog.LoggerFromContextOrDefault(ctx)
	var tx *sql.Tx
	var user users.User
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
	} else {
		if config.MasterEmail == "" || config.Bucket == "" {
			err = errors.New("A master email wasn't defined in arguments, please rerun with -master-email=<email> -bucket=<bucket-name>")
			logger.Error("Empty arguments master-email or bucket", err.Error())
		} else if user, err = setupMasterAccount(ctx, tx); err != nil {
			logger.Error("Failed to setup Master Account. ", err.Error())
		} else {
			logger.Info("Setup Master Account done.", user)
		}
	}
	return
}

func setupMasterAccount(ctx context.Context, tx *sql.Tx) (users.User, error) {
	logger := jsonlog.LoggerFromContextOrDefault(ctx)
	user, err := users.GetUserWithEmail(ctx, tx, config.MasterEmail)
	if err == users.ErrUserNotFound {
		randomPassowrd := generatePassword(config.SizePassword)
		user, err = users.CreateUserWithPassword(ctx, tx, config.MasterEmail, randomPassowrd, "customerIdentifier")
		if err != nil {
			return users.User{}, err
		}
		// send e-mail with the master account password
		err = mail.SendMail(
			config.MasterEmail,
			"Trackit It Master Account",
			"Congratulations!\nYour master account was created with success and your Password is: "+randomPassowrd,
			ctx)
		logger.Info("Master Trackit Account created.", nil)
	}

	if err != nil {
		return users.User{}, err
	}

	if err = createMasterAwsAccount(user, ctx, tx); err != nil {
		return users.User{}, err
	}

	return user, nil
}

func createMasterAwsAccount(user users.User, ctx context.Context, tx *sql.Tx) error {
	awsAccounts, err := aws.GetAwsAccountsFromUser(user, tx)
	logger := jsonlog.LoggerFromContextOrDefault(ctx)
	var awsAccount aws.AwsAccount
	if err == sql.ErrNoRows || len(awsAccounts) == 0 {
		awsAccount = aws.AwsAccount{
			UserId:   user.Id,
			RoleArn:  "no role",
			Pretty:   "tfg-trackit",
			External: "",
			Payer:    true,
		}

		err = awsAccount.CreateAwsAccount(ctx, tx)

	} else if len(awsAccounts) > 0 {
		awsAccount = awsAccounts[0]
	}

	if err != nil {
		return err
	}

	logger.Info("Setup Aws Account for Master Trackit account done.", awsAccount)

	return createMasterBill(ctx, awsAccount, tx)
}

func createMasterBill(ctx context.Context, aa aws.AwsAccount, tx *sql.Tx) error {
	logger := jsonlog.LoggerFromContextOrDefault(ctx)
	billRepositories, err := s3.GetBillRepositoriesForAwsAccount(aa, tx)

	var billRepository s3.BillRepository

	if err == sql.ErrNoRows || len(billRepositories) == 0 {
		billRepository, err = s3.CreateBillRepository(aa, s3.BillRepository{Prefix: "", Bucket: config.Bucket, AwsAccountId: aa.Id}, tx)
	} else if len(billRepositories) > 0 {
		billRepository = billRepositories[0]
	}

	if err != nil {
		return err
	}

	logger.Info("Setup Bill Repository for Master Trackit account done.", billRepository)
	return nil
}

// generatePassword generate random password to master account
func generatePassword(size int) string {
	const validChars string = "abcdefghijklmnopqrstuvyxwzABCDEFGHIJKLMNOPQRSTUYXWZ0123456789!@#$+=-"
	rand.Seed(time.Now().UnixNano())
	password := make([]byte, size)
	rand.Seed(time.Now().UnixNano())
	countChars := len(validChars)
	for i := 0; i < size; i++ {
		password[i] = validChars[rand.Intn(countChars)]
	}
	return string(password)
}
