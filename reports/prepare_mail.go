package reports

import (
	"bytes"
	//"database/sql"
	//"fmt"
	//"net/http"
	//"path"
	"sort"
	"strconv"
	"strings"
	"time"
	//"github.com/aws/aws-sdk-go/aws"
	//"github.com/aws/aws-sdk-go/service/s3"
	//"github.com/aws/aws-sdk-go/service/s3/s3manager"
	//"github.com/trackit/trackit-server/awsSession"
	//"github.com/trackit/trackit-server/config"
	//"github.com/trackit/trackit-server/db"
	//"github.com/trackit/trackit-server/models"
	//"github.com/trackit/trackit-server/routes"
	//"github.com/trackit/trackit-server/users"
	//"github.com/trackit/trackit-server/costs"

	tri "github.com/trackit/trackit-server/aws/ri"
	"github.com/trackit/trackit-server/costs/tags"
	"github.com/trackit/trackit-server/es"
	ts3 "github.com/trackit/trackit-server/s3/costs"
	"github.com/trackit/trackit-server/usageReports/ec2"
	"github.com/trackit/trackit-server/usageReports/ri"
)

type GeneralInformation struct {
	TotalActiveRI     int64   // #
	TotalActiveRICost float64 // $ invested

	TotalLowUsedInstances     int     // #
	TotalLowUsedInstancesCost float64 // $

	TotalUsageS3      float64 // GB
	TotalCostS3       float64 // $
	TotalInstancesS3  int     // #
	TotalDailyUsageS3 float64 // GB/day
	TotalDailyCostS3  float64 // $/day

	//TotalInstancesEc2 int // #
	//TotalInstancesRds int // #

	TotalCostProductEc2 float64 // $
	TotalCostProductRDS float64 // $

}

func formatEmail(reportRI ri.ResponseReservedInstance, reservedInstances []tri.ReservedInstance, s3info ts3.BucketsInfo, unusedInstances []ec2.InstanceReport, tagsValues tags.TagsValuesResponse, costReport es.SimplifiedCostsDocument, begin time.Time, end time.Time) (string, error) {
	email := `
	<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Transitional//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-transitional.dtd">
	<html xmlns="http://www.w3.org/1999/xhtml">
		<head>
			<meta http-equiv="Content-Type" content="text/html; charset=UTF-8" />
			<title>Demystifying Email Design</title>
			<meta name="viewport" content="width=device-width, initial-scale=1.0"/>
		</head>
		<body>
			<center><h1>Cost Report
`

	generalInformation := &GeneralInformation{}
	bucketsFormated := formatS3Buckets(s3info, generalInformation)
	reservedFormated := formatReserved(reservedInstances, generalInformation)
	unusedFormated := formatUnusedInstances(unusedInstances, generalInformation)
	tagsFormated := formatTags(tagsValues, generalInformation)
	instancesFormated := formatReportReserved(reportRI)

	generalFormated := formatGeneral(generalInformation, begin, end)

	email += begin.Format("2006-01-02")

	email += "</h1></center>"

	email += generalFormated

	email += "<hr /><h3>More information</h3>"

	email += bucketsFormated

	email += reservedFormated

	email += instancesFormated

	email += unusedFormated

	email += tagsFormated

	email += "<br />To complete detailed information, check the site report in http://trackit-client.apps.topaz-analytics.com/</body></html>"

	return email, nil
}

func formatGeneral(generalInformation *GeneralInformation, begin time.Time, end time.Time) string {

	var formated bytes.Buffer
	formated.WriteString("Data report " + begin.Format("2006-01-02") + " to " + end.Format("2006-01-02"))
	formated.WriteString("<table><tr>")
	formated.WriteString("<td><b>Active reserved instances</b></td>")
	formated.WriteString("<td>" + strconv.FormatInt(generalInformation.TotalActiveRI, 10) + "</td></tr>")
	formated.WriteString("<tr><td><b>Active reserved cost </b></td>")
	formated.WriteString("<td>$" + fToS(generalInformation.TotalActiveRICost) + "</td>")
	formated.WriteString("</tr> <tr><td>-</td><td></td></tr>")

	formated.WriteString("<tr>")
	formated.WriteString("<td><b>Low usage instances</b></td>")
	formated.WriteString("<td>" + strconv.Itoa(generalInformation.TotalLowUsedInstances) + "</td></tr>")
	formated.WriteString("<tr><td><b>Low used instances cost</b></td>")
	formated.WriteString("<td>$" + fToS(generalInformation.TotalLowUsedInstancesCost) + "</td>")
	formated.WriteString("</tr> <tr><td>-</td><td></td></tr>")

	formated.WriteString("<tr>")
	formated.WriteString("<td><b>Used S3 </b></td>")
	formated.WriteString("<td>" + fToS(generalInformation.TotalUsageS3) + "GB</td></tr>")
	formated.WriteString("<tr><td><b>Cost S3 </b></td>")
	formated.WriteString("<td>$" + fToS(generalInformation.TotalCostS3) + "</td>")
	formated.WriteString("</tr><tr>")
	formated.WriteString("<td><b>Used S3 per day</b></td>")
	formated.WriteString("<td>" + fToS(generalInformation.TotalDailyUsageS3) + "GB </td></tr>")
	formated.WriteString("<tr><td><b>Cost S3 per day</b></td>")
	formated.WriteString("<td>$ " + fToS(generalInformation.TotalDailyCostS3) + "</td>")
	formated.WriteString("</tr><tr><td><b>Instances S3 </b></td>")
	formated.WriteString("<td>" + strconv.Itoa(generalInformation.TotalInstancesS3) + "</td></tr>")
	formated.WriteString("<tr><td>-</td><td></td></tr>")

	formated.WriteString("<tr><td><b>EC2 Cost </b></td>")
	formated.WriteString("<td>$" + fToS(generalInformation.TotalCostProductEc2) + "</td></tr>")
	formated.WriteString("<tr><td><b>RDS Cost </b></td>")
	formated.WriteString("<td>$" + fToS(generalInformation.TotalCostProductRDS) + "</td></tr>")
	formated.WriteString("</table>")
	return formated.String()
}

func formatTags(tagsValues tags.TagsValuesResponse, generalInformation *GeneralInformation) string {
	ec2Application := make(map[string]float64)
	rdsApplication := make(map[string]float64)
	for _, tags := range tagsValues["Application,Owner"] {
		// Tag have the format [application, owner] and can be empty as [,]
		tag := strings.TrimSuffix(tags.Tag, "]")
		tag = strings.TrimPrefix(tag, "[")
		tag = strings.ToLower(tag)
		//tagGroups := strings.Split(tag, ",")
		//application := tagGroups[0]
		for _, cost := range tags.Costs {
			if cost.Item == "AmazonEC2" {
				ec2Application[tag] = cost.Cost
				generalInformation.TotalCostProductEc2 += cost.Cost
			} else if cost.Item == "AmazonRDS" {
				rdsApplication[tag] = cost.Cost
				generalInformation.TotalCostProductRDS += cost.Cost
			}
		}
	}

	keys := make([]string, 0, len(ec2Application))
	for key := range ec2Application {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return (ec2Application[keys[i]] + rdsApplication[keys[i]]) > (ec2Application[keys[j]] + rdsApplication[keys[j]])
	})

	var formated bytes.Buffer
	formated.WriteString("<h2>Applications EC2 and RDS cost</h2><table><tr><td>Application/Owner</td><td>EC2 Cost</td><td>RDS Cost</td></tr>")

	for i := 0; i < 15 && len(keys) > i; i++ {
		key := keys[i]
		formated.WriteString("<tr><td>" + key + "</td><td>" + fToS(ec2Application[key]) + "</td><td>" + fToS(rdsApplication[key]) + "</td></tr>")
	}
	formated.WriteString("</table>...<br />")
	return formated.String()
}

func formatReserved(reservedInstances []tri.ReservedInstance, generalInformation *GeneralInformation) string {
	// https://stackoverflow.com/questions/32751537/why-do-i-get-a-cannot-assign-error-when-setting-value-to-a-struct-as-a-value-i

	computational := make(map[string]float64)
	cost := make(map[string]float64)
	for _, instance := range reservedInstances {
		computational[instance.Family] += (instance.NormalizationFactor * float64(instance.InstanceCount))
		cost[instance.Family] += (float64(instance.InstanceCount) * instance.FixedPrice)
		generalInformation.TotalActiveRICost += (float64(instance.InstanceCount) * instance.FixedPrice)
		generalInformation.TotalActiveRI += instance.InstanceCount
	}

	var formated bytes.Buffer
	formated.WriteString("<h2>Reserved Instances</h2><table><tr><td>Family</td><td>Computational Units</td><td>Invested Value $</td></tr>")

	for family, reserved := range computational {
		formated.WriteString("<tr><td>" + family + "</td><td>" + fToS(reserved) + "</td><td>" + fToS(cost[family]) + "</td></tr>")
	}
	formated.WriteString("</table>")

	return formated.String()
}

func formatReportReserved(reportRI ri.ResponseReservedInstance) string {
	var formated bytes.Buffer
	formated.WriteString("<h2>Report usage of instances and discounted usages</h2><table><tr><td>Usage</td><td>Discount Usage</td></tr><tr><td>")
	formated.WriteString("<table><tr><td>Family</td><td>Total Cost</td><td>Usage</td><td>Cost per usage</td><td>Normalization Factor</td></tr>")
	formated.WriteString(getUsage(reportRI["Usage"]))
	formated.WriteString("</table></td><td>")
	formated.WriteString("<table><tr><td>Family</td><td>Total Discount</td><td>Usage</td><td>Cost per usage</td><td>Normalization Factor</td></tr>")
	formated.WriteString(getUsage(reportRI["DiscountedUsage"]))
	formated.WriteString("</td></tr></table> <br />...")

	return formated.String()
}

func getUsage(usages []tri.ReservedInstanceReport) string {
	var formated bytes.Buffer
	sort.Slice(usages, func(i, j int) bool {
		if usages[i].Family == usages[j].Family {
			return usages[i].NormalizedUsage > usages[j].NormalizedUsage
		}
		return usages[i].Family > usages[j].Family
	})
	for i := 0; i < 10 && len(usages) > i; i++ {
		usage := usages[i]
		if usage.Family != "" {
			unitCost := 0.0
			if usage.NormalizedUsage > 0 {
				unitCost = usage.Cost / usage.NormalizedUsage
			}
			formated.WriteString("<tr><td>" + usage.Family + "</td><td>" + fToS(usage.Cost) + "</td><td>" + fToS(usage.NormalizedUsage) + "</td><td>" + strconv.FormatFloat(unitCost, 'f', 4, 64) + "</td><td>" + fToS(usage.NormalizationFactor) + "</td></tr>")
		}
	}
	return formated.String()
}

func formatUnusedInstances(instances []ec2.InstanceReport, generalInformation *GeneralInformation) string {

	unusedAmount := make(map[string]float64)
	unusedCost := make(map[string]float64)
	unusedNames := make(map[string]string)
	generalInformation.TotalLowUsedInstances = len(instances)
	for _, instance := range instances {
		unusedAmount[instance.Instance.Type] += instance.Instance.NormalizationFactor
		unusedCost[instance.Instance.Type] += instance.Instance.Costs["instance"]
		unusedNames[instance.Instance.Type] += ("<br>" + instance.Instance.Tags["Name"] + "</br>")
		generalInformation.TotalLowUsedInstancesCost += instance.Instance.Costs["instance"]
	}

	var formated bytes.Buffer
	formated.WriteString("<h2>Low used instances</h2><table><tr><td>Family</td><td>Cost</td><td>Computational Power</td><td>Names</td></tr>")

	keys := make([]string, 0, len(unusedAmount))
	for key := range unusedAmount {
		keys = append(keys, key)
	}

	sort.Slice(keys, func(i, j int) bool {
		return unusedCost[keys[i]] > unusedCost[keys[j]]
	})

	for i := 0; i < 5 && len(keys) > i; i++ {
		key := keys[i]
		formated.WriteString("<tr><td><b>" + key + "</b></td><td>" + fToS(unusedCost[key]) + "</td><td>" + fToS(unusedAmount[key]) + "</td><td>" + unusedNames[key] + "</td></tr>")
	}
	formated.WriteString("</table><br />...")
	return formated.String()
}

func formatS3Buckets(buckets ts3.BucketsInfo, generalInformation *GeneralInformation) string {
	generalInformation.TotalInstancesS3 = len(buckets)
	var instanceBuckets []map[string]string
	for bucket, data := range buckets {
		generalInformation.TotalUsageS3 += data.GbMonth
		generalInformation.TotalCostS3 += (data.StorageCost + data.BandwidthCost + data.RequestsCost)
		value := 0.0
		if data.GbMonth > 0 {
			value = data.StorageCost / data.GbMonth
		}
		instanceBuckets = append(instanceBuckets, map[string]string{
			"name":    bucket,
			"total":   fToS(data.StorageCost + data.BandwidthCost + data.RequestsCost),
			"valueGb": fToS(value),
			"size":    fToS(data.GbMonth),
		})
	}
	generalInformation.TotalDailyUsageS3 = generalInformation.TotalUsageS3 / 30.4365
	generalInformation.TotalDailyCostS3 = generalInformation.TotalCostS3 / 30.4365

	sort.Slice(instanceBuckets, func(i, j int) bool {
		totalA, _ := strconv.ParseFloat(instanceBuckets[i]["total"], 64)
		totalB, _ := strconv.ParseFloat(instanceBuckets[j]["total"], 64)
		return totalA > totalB
	})

	var formated bytes.Buffer
	formated.WriteString("<h2>S3</h2><table><tr><td></td><td>Name</td><td>Total Cost $</td><td>Value (GB/$)</td><td>Month Size (GB)</td></tr>")
	for i := 0; i < 10 && len(instanceBuckets) > i; i++ {
		formated.WriteString("<tr><td>" + strconv.Itoa(i) + "</td><td><b>" + instanceBuckets[i]["name"] + "</b></td><td>" + instanceBuckets[i]["total"] + "</td><td>" + instanceBuckets[i]["valueGb"] + "</td><td>" + instanceBuckets[i]["size"] + "</td></tr>")
	}
	formated.WriteString("</table><br />...")
	return formated.String()
}

func fToS(float float64) string {
	return strconv.FormatFloat(float, 'f', 2, 64)
}
