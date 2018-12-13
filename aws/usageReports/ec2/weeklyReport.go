//   Copyright 2018 MSolution.IO
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

package ec2

import (
	"context"
	"time"

	"github.com/trackit/jsonlog"

	taws "github.com/trackit/trackit-server/aws"
	"github.com/trackit/trackit-server/aws/usageReports"
)

// PutEc2WeeklyReport puts a weekly report of EC2 instance in ES
func PutEc2WeeklyReport(ctx context.Context, ec2Cost, cloudWatchCost []utils.CostPerResource, aa taws.AwsAccount, startDate, endDate time.Time) error {
	logger := jsonlog.LoggerFromContextOrDefault(ctx)
	logger.Info("Starting EC2 weekly report", map[string]interface{}{
		"awsAccountId": aa.Id,
		"startDate":    startDate.Format("2006-01-02T15:04:05Z"),
		"endDate":      endDate.Format("2006-01-02T15:04:05Z"),
	})
	costInstance, costVolume, costCloudWatch := filterInstancesCosts(ec2Cost, cloudWatchCost)
	if len(costInstance) == 0 {
		logger.Info("No EC2 instances found in billing data.", nil)
		return nil
	}
	already, err := utils.CheckReportExists(ctx, startDate, aa, IndexPrefixEC2Report, utils.WEEKLY)
	if err != nil {
		return err
	} else if already {
		logger.Info("There is already an EC2 weekly report", nil)
		return nil
	}
	instances, err := fetchMonthlyInstancesStats(ctx, costInstance, aa, startDate, endDate, utils.WEEKLY)
	if err != nil {
		return err
	}
	instances = addCostToInstances(instances, costVolume, costCloudWatch)
	return importInstancesToEs(ctx, aa, instances)
}
