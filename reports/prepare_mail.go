package reports

import (
	"bytes"
	//"database/sql"
	//"fmt"
	//"net/http"
	//"path"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/message"

	"github.com/trackit/trackit-server/aws"
	"github.com/trackit/trackit-server/aws/product"
	tri "github.com/trackit/trackit-server/aws/ri"
	"github.com/trackit/trackit-server/aws/usageReports/rds"
	"github.com/trackit/trackit-server/costs/tags"
	"github.com/trackit/trackit-server/es"
	ts3 "github.com/trackit/trackit-server/s3/costs"
	"github.com/trackit/trackit-server/usageReports/ec2"
	trds "github.com/trackit/trackit-server/usageReports/rds"
	"github.com/trackit/trackit-server/usageReports/ri"
)

type (
	GeneralInformation struct {
		// Reservations
		TotalActiveRI      int64   // #
		TotalActiveRICost  float64 // $ invested
		ReservesWillExpire int     // $ invested

		// LowUsedInstances
		TotalLowUsedInstances        int     // #
		TotalLowUsedInatancesEc2Cost float64 // $
		HistogramUnusedEc2           Histogram
		LowUsedRds                   int // #

		// S3
		TotalUsageS3      float64 // GB
		TotalCostS3       float64 // $
		TotalInstancesS3  int     // #
		TotalDailyUsageS3 float64 // xB/day
		TotalDailyCostS3  float64 // $/day

		UsageProportion      float64 // %
		DiscountedProportion float64 // %
		//TotalInstancesEc2 int // #
		//TotalInstancesRds int // #

		// Ec2
		TotalCostInstancesEC2 float64      // $
		PowerProductEc2       []Ec2Product // %
		TotalPowerEc2         float64      // %
		TotalEc2Instances     int64        // #

		// RDS
		TotalCostInstancesRDS float64 // $
		LowUsedRdsCost        float64 // $
		TotalInstancesRds     int     // #
	}

	Histogram struct {
		min    float64
		max    float64
		values []int
		h      float64
		k      int
	}

	S3Product struct {
		name       string
		total      float64
		valueGb    float64
		size       float64
		simbolSize string
	}

	Ec2Product struct {
		Family string
		Value  float64
	}
)

func formatEmail(reportRI ri.ResponseReservedInstance, reservedInstances []tri.ReservedInstance, s3info ts3.BucketsInfo, unusedInstances []ec2.InstanceReport, tagsValues tags.ProductsTagsResponse, costReport es.SimplifiedCostsDocument, ec2Instances []ec2.InstanceReport, rdsInstances []rds.InstanceReport, productsPrice product.EC2ProductsPrice, begin time.Time, end time.Time) (string, error) {
	email := `
	<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Transitional//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-transitional.dtd">
	<html xmlns="http://www.w3.org/1999/xhtml">
		<head>
			<meta http-equiv="Content-Type" content="text/html; charset=UTF-8" />
			<title>Demystifying Email Design</title>
			<meta name="viewport" content="width=device-width, initial-scale=1.0"/>
		</head>
		<body>
			<center><h1>AWS Usage Report</h1></center>
`

	generalInformation := &GeneralInformation{}
	bucketsFormated := formatS3Buckets(s3info, generalInformation)
	reservedFormated := formatReserved(reservedInstances, generalInformation, begin, end)
	unusedFormated := formatUnusedInstances(unusedInstances, generalInformation)
	tagsFormated := formatProductsUsageInstances(tagsValues, generalInformation)
	instancesFormated := formatReportReserved(reportRI, productsPrice, begin, end, generalInformation)
	formatEc2Instances(ec2Instances, reservedInstances, generalInformation)
	unusedRdsFormated := formatUnusedRdsInstances(rdsInstances, generalInformation)

	generalInformation.HistogramUnusedEc2 = calculateHistogram(ec2Instances, 5)

	generalFormated := formatGeneral(generalInformation, begin, end)

	//email += begin.Format("2006-01-02")

	//email += "</h1></center>"

	email += generalFormated

	email += "<br /><hr /><br /><br /><h1>More information</h1><br /><br />"

	email += reservedFormated

	email += "<h2>Low Used Resources</h2><p>To calculate the low used resources, this report consider the <b>Average CPU</b> monthly usage lower than <b>10%</b> and the instance (RDS or EC2) with a peak of CPU usage lower than <b>60%</b>.</p><br />"
	email += unusedFormated
	email += unusedRdsFormated
	email += "<br /><a href=\"http://trackit-client.apps.topaz-analytics.com/app/resources\">Ver mais</a><br /><br /><hr /><br /><br />"

	email += bucketsFormated

	email += instancesFormated

	email += tagsFormated

	email += "<br />To complete detailed information, check the site report in http://trackit-client.apps.topaz-analytics.com/</body></html>"

	return email, nil
}

func formatGeneral(generalInformation *GeneralInformation, begin time.Time, end time.Time) string {

	var formated bytes.Buffer
	formated.WriteString("<table cellspacing=\"0\" cellpadding=\"5\"><tr>")
	formated.WriteString("<td><b>Date report</b></td>")
	formated.WriteString("<td>" + begin.Format("2006-01-02") + " ~ " + end.Format("2006-01-02") + "</td></tr>")

	formated.WriteString("<tr><td>-</td><td>-</td></tr>")

	formated.WriteString("<tr><td><b>Active EC2 Reserved Instances</b></td>")
	formated.WriteString("<td>" + strconv.FormatInt(generalInformation.TotalActiveRI, 10) + "</td></tr>")
	formated.WriteString("<td><b>Expiring EC2 Reserved Instances on next month<sup>1</sup></b></td>")
	formated.WriteString("<td>" + strconv.Itoa(generalInformation.ReservesWillExpire) + "</td></tr>")
	formated.WriteString("<tr><td><b>Active EC2 Reserved Instances Cost</b></td>")
	formated.WriteString("<td>$ " + fToS(generalInformation.TotalActiveRICost) + "</td></tr>")

	formated.WriteString("<tr><td>-</td><td>-</td></tr>")

	formated.WriteString("<tr><td><b>EC2 Instances <sup>2</sup></b></td>")
	formated.WriteString("<td>" + strconv.FormatInt(generalInformation.TotalEc2Instances, 10) + "</td></tr>")
	formated.WriteString("<tr><td><b>EC2 Instances Cost</b></td>")
	formated.WriteString("<td>$ " + fToS(generalInformation.TotalCostInstancesEC2) + "</td></tr>")

	formated.WriteString("<tr><td><b>EC2 Computational Power Proportion <sup>3</sup></b></td>")
	formated.WriteString("<td><table><tr><th width=\"100px\">On Demand</th><th width=\"100px\">Reserved</th></tr>")
	formated.WriteString("<tr><td>" + fToS(generalInformation.UsageProportion) + "%</td>")
	formated.WriteString("<td>" + fToS(generalInformation.DiscountedProportion) + "%</td></tr></table>")
	formated.WriteString("</td></tr>")

	formated.WriteString("<tr><td><b>Distribution of EC2 Instances <br />by Family <sup>4</sup></b></td>")
	formated.WriteString("<td> <table cellspacing=\"0\" cellpadding=\"5\"><tr><td><b>Family</b></td><td><b>Power</b></td></tr>")
	for _, product := range generalInformation.PowerProductEc2 {
		formated.WriteString("<tr><td>" + product.Family + "</td><td>" + fToS(product.Value) + " %</td></tr>")
	}
	formated.WriteString("</td></tr>")

	formated.WriteString("<tr><td><b>Histogram of percentages of <br />the monthly average of CPU usage</b></td>")
	formated.WriteString("<td>" + formatHistogram(generalInformation.HistogramUnusedEc2) + "</td></tr>")
	formated.WriteString("<tr><td><b>Low Used EC2 Instances Cost <sup>5</sup></b></td>")
	formated.WriteString("<td>$ " + fToS(generalInformation.TotalLowUsedInatancesEc2Cost) + "</td></tr>")

	formated.WriteString("<tr><td>-</td><td></td></tr>")

	formated.WriteString("</tr><tr><td><b>S3 Buckets</b></td>")
	formated.WriteString("<td>" + strconv.Itoa(generalInformation.TotalInstancesS3) + "</td></tr>")
	formated.WriteString("<tr><td><b>S3 Buckets Cost</b></td>")
	formated.WriteString("<td>$ " + fToS(generalInformation.TotalCostS3) + "</td></tr>")
	formated.WriteString("<tr><td><b>S3 Storage Usage</b></td>")
	simbol, value := formatGb(generalInformation.TotalUsageS3)
	formated.WriteString("<td>" + fToS(value) + simbol + "</td></tr>")
	formated.WriteString("<tr><td><b>S3 Storage Usage Per Day</b></td>")
	simbol, value = formatGb(generalInformation.TotalDailyUsageS3)
	formated.WriteString("<td>" + fToS(value) + simbol + "</td></tr>")
	//formated.WriteString("<tr><td><b>S3 Cost Per Day</b></td>")
	//formated.WriteString("<td>$ " + fToS(generalInformation.TotalDailyCostS3) + "</td></td>")

	formated.WriteString("<tr><td>-</td><td></td></tr>")

	formated.WriteString("<tr><td><b>RDS Instances</b></td>")
	formated.WriteString("<td>" + strconv.Itoa(generalInformation.TotalInstancesRds) + "</td></tr>")
	formated.WriteString("<tr><td><b>RDS Cost <sup>6</sup></b></td>")
	formated.WriteString("<td>$ " + fToS(generalInformation.TotalCostInstancesRDS) + "</td></tr>")
	formated.WriteString("<td><b>RDS Low Used Instances <sup>5</sup></b></td>")
	formated.WriteString("<td>" + strconv.Itoa(generalInformation.LowUsedRds) + "</td></tr>")
	formated.WriteString("<tr><td><b>RDS Low Used Instances Cost</b></td>")
	formated.WriteString("<td>$ " + fToS(generalInformation.LowUsedRdsCost) + "</td></tr>")
	formated.WriteString("</table><br />")

	formated.WriteString("<sup>1</sup> The total number of machines that will expire in the next month.<br />")
	formated.WriteString("<sup>2</sup> Only instances that generate some cost report in the bill of this month is considered in this count, so it doens't overleap the number of active reserved instances.<br />")
	formated.WriteString("<sup>3</sup> The proportion of computational power calculate the normalized usage of different instances types in hours during this interval of report.<br />")
	formated.WriteString("<sup>4</sup> The distribution of instances by family regarding the sum of compute units (ECU) of all instances.<br />")
	formated.WriteString("<sup>5</sup> Low used instances consider average CPU usage lower than 10% and no peaks of use over 60%.")
	formated.WriteString("<sup>6</sup> The instance and storage usage are used to calculate the RDS cost.<br /><br />")
	return formated.String()
}

func formatProductsUsageInstances(tagsValues tags.ProductsTagsResponse, generalInformation *GeneralInformation) string {

	for _, tag_group := range tagsValues {
		generalInformation.TotalCostInstancesEC2 += tag_group.Ec2Cost
		generalInformation.TotalCostInstancesRDS += tag_group.RdsCost
	}

	sort.Slice(tagsValues, func(i, j int) bool {
		return tagsValues[i].Ec2Cost > tagsValues[j].Ec2Cost
	})

	var formated bytes.Buffer
	formated.WriteString("<h2>More expensives usages for EC2</h2><br /><br />These are applications are the most expensive applications regarding EC2 Product cost. <br /><br /><br />")
	formated.WriteString("<table width=\"600px\" cellspacing=\"0\" cellpadding=\"5\"><tr><td></td><td><b>Application</b></td><td><b>Owner</b></td><td><b>EC2 Cost</b></td></tr>")

	for i := 0; i < 10 && len(tagsValues) > i; i++ {
		item := tagsValues[i]

		formated.WriteString("<tr><td>" + strconv.Itoa(i+1) + "</td><td>" + item.Application + "</td><td>" + item.Owner + "</td><td>$ " + fToS(item.Ec2Cost) + "</td></tr>")
	}
	formated.WriteString("</table><a href=\"http://trackit-client.apps.topaz-analytics.com/app/tags\">Ver mais</a><br />")
	return formated.String()

}

// deprecated
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
				//generalInformation.TotalCostProductEc2 += cost.Cost
			} else if cost.Item == "AmazonRDS" {
				rdsApplication[tag] = cost.Cost
				//generalInformation.TotalCostProductRDS += cost.Cost
			}
		}
	}
	return ""
}

// deprecated
func splitApplicationOwner(key string) (string, string) {
	splitedKey := strings.Split(key, ",")
	if len(splitedKey) != 2 {
		return "No tag", "No tag"
	}
	if strings.Trim(splitedKey[0], " ") == "" {
		splitedKey[0] = "No tag"
	}

	if strings.Trim(splitedKey[1], " ") == "" {
		splitedKey[1] = "No tag"
	}
	return splitedKey[0], splitedKey[1]
}

func formatReserved(reservedInstances []tri.ReservedInstance, generalInformation *GeneralInformation, startDate, endDate time.Time) string {
	// https://stackoverflow.com/questions/32751537/why-do-i-get-a-cannot-assign-error-when-setting-value-to-a-struct-as-a-value-i

	//computational := make(map[string]float64)

	expireReserve := make(map[string]int)
	expireReserveComputational := make(map[string]float64)
	expireReserveDate := make(map[string]map[string]int)

	cost := make(map[string]float64)

	nextReport := time.Date(endDate.Year(), endDate.Month()+2, 0, 23, 59, 59, 999999999, time.Local)

	for _, instance := range reservedInstances {
		//computational[instance.Family] += (instance.NormalizationFactor * float64(instance.InstanceCount))
		generalInformation.TotalActiveRICost += (float64(instance.InstanceCount) * instance.FixedPrice)
		generalInformation.TotalActiveRI += instance.InstanceCount
		if instance.EndDate.Before(nextReport) {
			cost[instance.InstanceType] += (float64(instance.InstanceCount) * instance.FixedPrice)
			expireReserve[instance.InstanceType] += int(instance.InstanceCount)
			expireReserveComputational[instance.InstanceType] += (float64(instance.InstanceCount) * instance.NormalizationFactor)
			generalInformation.ReservesWillExpire += int(instance.InstanceCount)

			if len(expireReserveDate[instance.InstanceType]) == 0 {
				expireReserveDate[instance.InstanceType] = make(map[string]int)
			}
			expireReserveDate[instance.InstanceType][instance.EndDate.Format("2006-01-02")] += 1
		}
	}

	if len(expireReserve) == 0 {
		return "<h2>No EC2 Reserved instances will expire until <b>" + nextReport.Format("2006-01-02") + "</b>.</h2>"
	}

	var formated bytes.Buffer
	formated.WriteString("<h2>Expirations of EC2 Reserved Instances</h2><br />The table below present only instances will expire until <b>" + nextReport.Format("2006-01-02") + "</b><br /> <br />")
	formated.WriteString("<table cellspacing=\"0\" cellpadding=\"5\"><tr><td width=\"100px\"><b>Instance Type</b></td><td width=\"200px\"><b>Expiring Instances</b></td><td width=\"100px\"><b>Date Expiration</b></td><td width=\"200px\"><b>Expiring Computational Power</b></td></tr>")

	for instanceType, _ := range expireReserve {

		// colect sorted keys
		var keys []string
		for key := range expireReserveDate[instanceType] {
			keys = append(keys, key)
		}
		sort.Sort(sort.StringSlice(keys))

		formated.WriteString("<tr><td>" + instanceType + "</td><td>" + strconv.Itoa(expireReserve[instanceType]) + "</td><td>" + strings.Join(keys, "<br />") + "</td><td>" + fToS(expireReserveComputational[instanceType]) + "</td></tr>")
	}
	formated.WriteString("</table><br /><a href=\"http://trackit-client.apps.topaz-analytics.com/app/reserves\">Ver mais</a><br />")

	return formated.String()
}

func formatReportReserved(reportRI ri.ResponseReservedInstance, productsPrice product.EC2ProductsPrice, startDate, endDate time.Time, generalInformation *GeneralInformation) string {
	totalUsages := make(map[string]float64)
	for key, report := range reportRI {
		for _, usage := range report {
			totalUsages[key] += usage.NormalizationFactor
		}
	}

	var formated bytes.Buffer

	total := totalUsages["Usage"] + totalUsages["DiscountedUsage"]
	if total > 0 {
		generalInformation.UsageProportion = (totalUsages["Usage"] / total) * 100
		generalInformation.DiscountedProportion = (totalUsages["DiscountedUsage"] / total) * 100
	}

	formated.WriteString(getUsage(reportRI["Usage"], productsPrice, startDate, endDate))
	//formated.WriteString("<p>* This recommendations are a simple analysis about usage in the reporting period and does not consider the number of used machines or average CPU / MEMORY / DISK / NETWORK usages.</p>")

	return formated.String()
}

func getUsage(usages []tri.ReservedInstanceReport, productsPrice product.EC2ProductsPrice, startDate, endDate time.Time) string {
	var formated bytes.Buffer

	formated.WriteString("<h2>On Demand EC2 Instances that can be reserved</h2>")
	formated.WriteString("These machines are <b>On Demand</b> usage and can be replaced by Reserved Instances.<br /><br /><br />")
	formated.WriteString("<table cellspacing=\"0\" cellpadding=\"5\"><tr><td></td><td width=\"120px\"><b>Instance Type</b></td><td><b>Machines <sup>1</sup></b></td><td width=\"200px\"><b>Cost On Demand</b></td><td><b>Reservation Could Reduce Cost To <sup>2</sup></b></td><td><b>Difference of</b></td></tr>")

	sort.Slice(usages, func(i, j int) bool {
		return usages[i].Cost > usages[j].Cost
	})

	interval := endDate.Sub(startDate)

	suggestionsCount := 0
	for i := 0; suggestionsCount < 7 && len(usages) > i; i++ {
		usage := usages[i]
		instanceType := usage.Family + "." + aws.InverseNormalizationFactor(usage.NormalizationFactor)

		if usage.Family != "" && productsPrice[instanceType] > 0 && interval.Hours() > 0 && usage.NormalizationFactor > 0 {
			// estimate the number of used machines based on total hours used of a single machine
			hoursUsage := usage.NormalizedUsage / usage.NormalizationFactor
			machines := math.Ceil(hoursUsage / float64(interval.Hours()))

			riCost := machines * productsPrice[instanceType] * interval.Hours()

			if usage.Cost-riCost > 0 {
				suggestionsCount++
				formated.WriteString("<tr><td>" + strconv.Itoa(suggestionsCount) + "</td><td>" + instanceType + "</td>")
				formated.WriteString("<td>" + strconv.Itoa(int(machines)) + "</td>")
				formated.WriteString("<td>$ " + fToS(usage.Cost) + "</td>")
				formated.WriteString("<td>$ " + fToS(riCost) + "</td>")
				formated.WriteString("<td>" + fToS(100.0*(usage.Cost-riCost)/usage.Cost) + "%</td>")
				formated.WriteString("</tr>")
			}
		}
	}
	formated.WriteString("</table><br /><a href=\"http://trackit-client.apps.topaz-analytics.com/app/usages\">Ver mais</a>")
	formated.WriteString("<br /><sup>1</sup> The number of machines is an approximation based on total hours of use for each instance type and the time interval of this report.<br />")
	formated.WriteString("<sup>2</sup> Using the machines estimation, we consider a <b>All Upfront</b> reservation machine from the same instance type, which <b>is not convertible</b> and does <b>not have</b> any <b>software preinstalled</b> from <b>US-East (N. Virginia)</b> to calculate the reservation cost.<br /><br />")

	if suggestionsCount == 0 {
		return "<h2>There are no suggestion to reserve EC2 instances</h2>This report doens't find a good replacement for usage of On Demand EC2 Instances.<br /><br />"
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
		for _, value := range instance.Instance.Costs {
			unusedCost[instance.Instance.Type] += value
			generalInformation.TotalLowUsedInatancesEc2Cost += value
		}
		unusedNames[instance.Instance.Type] += (instance.Instance.Tags["Name"] + "<br />")
	}

	var formated bytes.Buffer
	formated.WriteString("<h4>EC2 Instances</h4><table width=\"600px\" cellspacing=\"0\" cellpadding=\"5\"><tr><th width=\"100px\" style=\"border-bottom: 1px solid; text-align: left;\">Family</th><th width=\"100px\" style=\"border-bottom: 1px solid; text-align: left;\">Cost</th><th style=\"border-bottom: 1px solid; text-align: left;\">Names</th></tr>")

	keys := make([]string, 0, len(unusedAmount))
	for key := range unusedAmount {
		keys = append(keys, key)
	}

	sort.Slice(keys, func(i, j int) bool {
		return unusedCost[keys[i]] > unusedCost[keys[j]]
	})

	for i := 0; i < 5 && len(keys) > i; i++ {
		key := keys[i]
		formated.WriteString("<tr><td style=\"border-bottom: 1px dashed;\"><b>" + key + "</b></td><td style=\"border-bottom: 1px dashed;\">" + fToS(unusedCost[key]) + "</td><td style=\"border-bottom: 1px dashed;\">" + unusedNames[key] + "</td></tr>")
	}
	formated.WriteString("</table><br />")
	return formated.String()
}

func formatUnusedRdsInstances(instances []rds.InstanceReport, generalInformation *GeneralInformation) string {

	unusedCost := make(map[string]float64)
	unusedPower := make(map[string]float64)
	unusedNames := make(map[string]string)

	for _, instance := range instances {
		if trds.IsInstanceUnused(instance.Instance) {
			generalInformation.LowUsedRds += 1
			for _, value := range instance.Instance.Costs {
				unusedCost[instance.Instance.DBInstanceClass] += value
				generalInformation.LowUsedRdsCost += value
			}
			unusedPower[instance.Instance.DBInstanceClass] += instance.Instance.NormalizationFactor
			unusedNames[instance.Instance.DBInstanceClass] += (instance.Instance.DBInstanceIdentifier + "<br /> ")
		}
		generalInformation.TotalInstancesRds += 1
	}

	keys := make([]string, 0, len(unusedPower))
	for key := range unusedNames {
		keys = append(keys, key)
	}

	sort.Slice(keys, func(i, j int) bool {
		return unusedCost[keys[i]] > unusedCost[keys[j]]
	})
	var formated bytes.Buffer

	formated.WriteString("<br /><br /><br /><h4>RDS Instances</h4><table width=\"600px\" cellspacing=\"0\" cellpadding=\"5\"><tr><th width=\"100px\" style=\"border-bottom: 1px solid; text-align: left;\">Instance Type</th><th width=\"100px\" style=\"border-bottom: 1px solid; text-align: left;\">Cost</th><th style=\"border-bottom: 1px solid; text-align: left;\">Names</th></tr>")

	for i := 0; i < 5 && len(keys) > i; i++ {
		key := keys[i]
		formated.WriteString("<tr><td style=\"border-bottom: 1px dashed;\"><b>" + key + "</b></td><td style=\"border-bottom: 1px dashed;\">" + fToS(unusedCost[key]) + "</td><td style=\"border-bottom: 1px dashed;\">" + unusedNames[key] + "</td></tr>")
	}
	formated.WriteString("</table>")

	return formated.String()
}

func formatEc2Instances(instances []ec2.InstanceReport, reservedInstances []tri.ReservedInstance, generalInformation *GeneralInformation) {

	ec2Power := make(map[string]float64)
	total := 0.0

	for _, instance := range instances {
		ec2Power[instance.Instance.Family] += instance.Instance.NormalizationFactor
		total += instance.Instance.NormalizationFactor
		generalInformation.TotalEc2Instances += 1
	}

	for _, instance := range reservedInstances {
		ec2Power[instance.Family] += (float64(instance.InstanceCount) * instance.NormalizationFactor)
		total += (float64(instance.InstanceCount) * instance.NormalizationFactor)
	}

	if total > 0 {
		for family, normalization := range ec2Power {
			product := Ec2Product{
				Family: family,
				Value:  100 * (normalization / total),
			}
			generalInformation.PowerProductEc2 = append(generalInformation.PowerProductEc2, product)
		}
		sort.Slice(generalInformation.PowerProductEc2, func(i, j int) bool {
			return generalInformation.PowerProductEc2[i].Value > generalInformation.PowerProductEc2[j].Value
		})
	}

}

func formatHistogram(histogram Histogram) string {
	var formated bytes.Buffer
	formated.WriteString("<table cellspacing=\"0\" cellpadding=\"5\"><tr><td><b>Average CPU</b></td><th>Instances</th></tr>")
	for x := 0; x < histogram.k; x++ {
		interval := 0.01
		if x == (histogram.k - 1) { // the last item of range is included
			interval = 0.0
		}

		lowerBound := math.Max(histogram.min+histogram.h*float64(x), 0.0)
		upperBound := math.Min(histogram.min+histogram.h*float64(x+1)-interval, 100.0)
		formated.WriteString("<tr>")
		formated.WriteString("<td width=\"150px\" >" + fToS(lowerBound) + "% ~ " + fToS(upperBound) + "%</td>")
		formated.WriteString("<td>" + strconv.Itoa(histogram.values[x]) + "</td>")
		formated.WriteString("</tr>")
	}
	formated.WriteString("</table>")
	return formated.String()
}

func calculateHistogram(instances []ec2.InstanceReport, k int) Histogram {
	min := 100.1
	max := 0.0
	// search for minmax
	for _, instance := range instances {
		if min > instance.Instance.Stats.Cpu.Average {
			min = instance.Instance.Stats.Cpu.Average
		}
		if max < instance.Instance.Stats.Cpu.Average {
			max = instance.Instance.Stats.Cpu.Average
		}
	}

	h := (max - min) / float64(k)
	histValues := make([]int, k)
	for _, instance := range instances {
		bucket := int((instance.Instance.Stats.Cpu.Average - min) / h)
		if bucket >= k {
			bucket = k - 1
		}
		histValues[bucket]++
	}

	return Histogram{
		min:    min,
		max:    max,
		values: histValues,
		h:      h,
		k:      k,
	}
}

func formatS3Buckets(buckets ts3.BucketsInfo, generalInformation *GeneralInformation) string {

	generalInformation.TotalInstancesS3 = len(buckets)

	var instanceBuckets []S3Product

	for bucket, data := range buckets {
		generalInformation.TotalUsageS3 += data.GbMonth
		generalInformation.TotalCostS3 += (data.StorageCost + data.BandwidthCost + data.RequestsCost)
		value := 0.0
		if data.GbMonth > 0 {
			value = data.StorageCost / data.GbMonth
		}
		simbolTotal, valueTotal := formatGb(data.GbMonth)
		instanceBuckets = append(instanceBuckets, S3Product{
			name:       bucket,
			total:      data.StorageCost + data.BandwidthCost + data.RequestsCost,
			valueGb:    value,
			size:       valueTotal,
			simbolSize: simbolTotal,
		})
	}

	generalInformation.TotalDailyUsageS3 = generalInformation.TotalUsageS3 / 30.4365
	generalInformation.TotalDailyCostS3 = generalInformation.TotalCostS3 / 30.4365

	sort.Slice(instanceBuckets, func(i, j int) bool {
		return instanceBuckets[i].total > instanceBuckets[j].total
	})

	var formated bytes.Buffer

	formated.WriteString("<h2>Expensive S3 Buckets</h2>These are the top five more expensive buckets used in the time interval of this report.")
	formated.WriteString("<table width=\"600px\" cellspacing=\"0\" cellpadding=\"5\"><tr><td></td><td><b>Name</b></td><td><b>Total Cost</b></td><td><b>Value (GB/$)</b></td><td><b>Month Size</b></td></tr>")

	for i := 0; i < 5 && len(instanceBuckets) > i; i++ {
		formated.WriteString("<tr><td>" + strconv.Itoa(i+1) + "</td>")
		formated.WriteString("<td>" + instanceBuckets[i].name + "</td>")
		formated.WriteString("<td>$ " + fToS(instanceBuckets[i].total) + "</td>")
		formated.WriteString("<td>" + strconv.FormatFloat(instanceBuckets[i].valueGb, 'f', 4, 64) + "</td>")
		formated.WriteString("<td>" + fToS(instanceBuckets[i].size) + instanceBuckets[i].simbolSize + "</td></tr>")
	}
	formated.WriteString("</table><br /><a href=\"http://trackit-client.apps.topaz-analytics.com/app/s3\">Ver mais</a>")
	return formated.String()
}

func formatGb(value float64) (string, float64) {
	formats := []string{"B", "KB", "MB", "GB", "TB", "PT", "EB", "ZB"}

	byteValue := value * 1024 * 1024 * 1024
	i := 0
	for byteValue/1024 >= 1 {
		byteValue /= 1024
		i++
	}

	return formats[i], byteValue
}

func fToS(float float64) string {
	printer := message.NewPrinter(message.MatchLanguage("en"))
	return printer.Sprintf("%-.2f", float)
}
