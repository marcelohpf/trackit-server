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

package reports

import (
	"database/sql"
	"fmt"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"

	"github.com/trackit/trackit-server/aws/product"
	"github.com/trackit/trackit-server/awsSession"
	"github.com/trackit/trackit-server/config"
	"github.com/trackit/trackit-server/db"
	"github.com/trackit/trackit-server/mail"
	"github.com/trackit/trackit-server/models"
	"github.com/trackit/trackit-server/routes"
	"github.com/trackit/trackit-server/users"

	"github.com/trackit/trackit-server/costs"
	"github.com/trackit/trackit-server/costs/tags"
	ts3 "github.com/trackit/trackit-server/s3/costs"
	"github.com/trackit/trackit-server/usageReports/ec2"
	"github.com/trackit/trackit-server/usageReports/rds"
	"github.com/trackit/trackit-server/usageReports/ri"
)

var reportMailArgs = []routes.QueryArg{
	routes.AwsAccountsOptionalQueryArg,
	routes.DateBeginQueryArg,
	routes.DateEndQueryArg,
	routes.QueryArg{
		Name:        "targets",
		Type:        routes.QueryArgStringSlice{},
		Description: "Targets of mail comma separed",
		Optional:    false,
	},
}

type (
	Report struct {
		content []byte
		name    string
	}

	ReportEmailQueryParams struct {
		accountList []string
		begin       time.Time
		end         time.Time
		indexList   []string
		targetList  []string
		state       string
	}
)

func init() {
	routes.MethodMuxer{
		http.MethodGet: routes.H(getAwsReports).With(
			db.RequestTransaction{Db: db.Db},
			users.RequireAuthenticatedUser{users.ViewerAsParent},
			routes.Documentation{
				Summary:     "get the list of aws reports",
				Description: "Responds with the list of reports based on the queryparams passed to it",
			},
			routes.QueryArgs{routes.AwsAccountIdQueryArg},
		),
	}.H().Register("/reports")

	routes.MethodMuxer{
		http.MethodGet: routes.H(getAwsReportsDownload).With(
			db.RequestTransaction{Db: db.Db},
			users.RequireAuthenticatedUser{users.ViewerAsParent},
			routes.Documentation{
				Summary:     "get an aws cost report spreadsheet",
				Description: "Responds with the spreadsheet based on the queryparams passed to it",
			},
			routes.QueryArgs{routes.AwsAccountIdQueryArg},
			routes.QueryArgs{routes.ReportTypeQueryArg},
			routes.QueryArgs{routes.FileNameQueryArg},
		),
	}.H().Register("/report")

	routes.MethodMuxer{
		http.MethodPost: routes.H(sendReportMail).With(
			db.RequestTransaction{Db: db.Db},
			users.RequireAuthenticatedUser{users.ViewerAsParent},
			routes.Documentation{
				Summary:     "send a report mail",
				Description: "Prepare and send a compact report with the main and necessary information to overview of aws cost",
			},
			routes.QueryArgs(reportMailArgs),
		),
	}.H().Register("/report/mail")
}

func isUserAccount(tx *sql.Tx, user users.User, aa int) (bool, error) {
	aaDB, err := models.AwsAccountByID(tx, aa)
	if err != nil {
		return false, err
	}
	if aaDB.UserID == user.Id {
		return true, nil
	}
	saDB, err := models.SharedAccountsByAccountID(tx, aa)
	if err != nil {
		return false, err
	}
	for _, key := range saDB {
		if key.UserID == user.Id {
			return true, nil
		}
	}
	return false, nil
}

// getAwsReports returns the list of reports based on the query params, in JSON format.
// The endpoint returns a list of strings following this format: report-type/file-name
func getAwsReports(request *http.Request, a routes.Arguments) (int, interface{}) {
	if config.ReportsBucket == "" {
		return http.StatusInternalServerError, fmt.Errorf("Reports bucket not configured")
	}
	user := a[users.AuthenticatedUser].(users.User)
	aa := a[routes.AwsAccountIdQueryArg].(int)
	tx := a[db.Transaction].(*sql.Tx)
	if aaOk, aaErr := isUserAccount(tx, user, aa); !aaOk {
		return http.StatusUnauthorized, aaErr
	}
	svc := s3.New(awsSession.Session)
	prefix := fmt.Sprintf("%d/", aa)
	objects := []string{}
	err := svc.ListObjectsPagesWithContext(request.Context(), &s3.ListObjectsInput{
		Bucket: aws.String(config.ReportsBucket),
		Prefix: aws.String(prefix),
	}, func(p *s3.ListObjectsOutput, lastPage bool) bool {
		for _, o := range p.Contents {
			// Do not keep folders
			if !strings.HasSuffix(*o.Key, "/") {
				objects = append(objects, strings.TrimPrefix(aws.StringValue(o.Key), prefix))
			}
		}
		return true // continue paging
	})
	if err != nil {
		return http.StatusInternalServerError, err
	}
	return http.StatusOK, objects
}

func (r Report) GetFileContent() []byte {
	return r.content
}

func (r Report) GetFileName() string {
	return r.name
}

// getAwsReportsDownload returns the report based on the query params, in excel format.
func getAwsReportsDownload(request *http.Request, a routes.Arguments) (int, interface{}) {
	if config.ReportsBucket == "" {
		return http.StatusInternalServerError, fmt.Errorf("Reports bucket not configured")
	}
	user := a[users.AuthenticatedUser].(users.User)
	aa := a[routes.AwsAccountIdQueryArg].(int)
	reportType := a[routes.ReportTypeQueryArg].(string)
	reportName := a[routes.FileNameQueryArg].(string)
	tx := a[db.Transaction].(*sql.Tx)
	if aaOk, aaErr := isUserAccount(tx, user, aa); !aaOk {
		return http.StatusUnauthorized, aaErr
	}
	reportPath := path.Join(strconv.Itoa(aa), reportType, reportName)
	buff := &aws.WriteAtBuffer{}
	downloader := s3manager.NewDownloader(awsSession.Session)
	_, err := downloader.Download(buff,
		&s3.GetObjectInput{
			Bucket: aws.String(config.ReportsBucket),
			Key:    aws.String(reportPath),
		})
	if err != nil {
		return http.StatusNotFound, fmt.Errorf("The specified key does not exist")
	}
	return http.StatusOK, Report{buff.Bytes(), reportName}
}

func getWeekDates(date time.Time) (time.Time, time.Time) {
	dayOfWeek := int(date.Weekday())
	begin := time.Date(date.Year(), date.Month(), date.Day()-dayOfWeek-7, 0, 0, 0, 0, date.Location()).UTC()
	end := time.Date(date.Year(), date.Month(), date.Day()-dayOfWeek-1, 23, 59, 59, 0, date.Location()).UTC()
	return begin, end
}

// sendReportMail collect data from endpoints and format a e-mail to send to a target list
func sendReportMail(request *http.Request, a routes.Arguments) (int, interface{}) {
	user := a[users.AuthenticatedUser].(users.User)
	tx := a[db.Transaction].(*sql.Tx)
	begin, end := getWeekDates(a[routes.DateBeginQueryArg].(time.Time))
	parsedParams := ReportEmailQueryParams{
		begin:       begin,
		end:         end,
		targetList:  a[reportMailArgs[3]].([]string),
		accountList: []string{},
	}

	ctx := request.Context()

	if a[routes.AwsAccountsOptionalQueryArg] != nil {
		parsedParams.accountList = a[routes.AwsAccountIdsOptionalQueryArg].([]string)
	}

	// fetch reserved instances
	ec2RiParams := ri.GetRiParams(parsedParams.begin, parsedParams.end, parsedParams.accountList, "active")

	returnCode, ec2Ri, err := ri.GetEC2ReservedInstances(ctx, ec2RiParams, user, tx)
	if err != nil {
		return returnCode, err
	}

	// fetch reserved instances report
	riReportParams := ri.GetRiReportParams(parsedParams.begin, parsedParams.end, parsedParams.accountList)

	returnCode, ec2RiReport, err := ri.GetRC2ReportReservedInstances(ctx, riReportParams, user, tx)
	if err != nil {
		return returnCode, err
	}

	// fetch costs
	costParams := costs.GetEsQueryParams(parsedParams.begin, parsedParams.end, []string{"product"}, parsedParams.accountList)
	returnCode, esCost, err := costs.GetCostData(ctx, costParams, user, tx)

	if err != nil {
		return returnCode, err
	}

	// fetch tags
	tagsParams := tags.Ec2TagsValuesQueryParams{
		AccountList: parsedParams.accountList,
		DateBegin:   parsedParams.begin,
		DateEnd:     parsedParams.end,
	}

	returnCode, tagsGroup, err := tags.GetGroupedTags(ctx, tagsParams, user, tx)

	if err != nil {
		return returnCode, err
	}

	// fetch S3
	s3Params := ts3.GetEsQueryParams(parsedParams.begin, parsedParams.end, parsedParams.accountList)

	returnCode, s3Cost, err := ts3.GetS3CostData(ctx, s3Params, user, tx)

	if err != nil {
		return returnCode, err
	}

	// fetch ec2 instances
	ec2Params := ec2.GetEc2QueryParams(parsedParams.accountList, parsedParams.begin, "weekly")

	returnCode, ec2Instances, err := ec2.GetEc2Data(ctx, ec2Params, user, tx)
	if err != nil {
		return http.StatusInternalServerError, err
	}

	// fetch rds instances
	rdsParams := rds.GetRdsQueryParams(parsedParams.accountList, parsedParams.begin, "weekly")

	returnCode, rdsInstances, err := rds.GetRdsData(ctx, rdsParams, user, tx)
	if err != nil {
		return http.StatusInternalServerError, err
	}

	productsPrice, err := product.GetProductsEC2HourlyPrice(ctx)

	if err != nil {
		return http.StatusInternalServerError, err
	}

	content, err := formatEmail(ec2RiReport, ec2Ri, s3Cost, tagsGroup, esCost, ec2Instances, rdsInstances, productsPrice, parsedParams.begin, parsedParams.end)
	if err != nil {
		return http.StatusInternalServerError, err
	}

	// sending mail
	err = mail.SendHTMLMail(parsedParams.targetList, "AWS Usage Report", content, ctx)
	if err != nil {
		return http.StatusInternalServerError, err
	}

	return http.StatusOK, map[string]string{"data": "email was sent with success"}
}
