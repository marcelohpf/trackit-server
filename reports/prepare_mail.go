package reports

import (
	"bytes"
	"math"
	"sort"
	"strconv"
	"time"

	"github.com/trackit/trackit-server/aws/product"
	tri "github.com/trackit/trackit-server/aws/ri"
	"github.com/trackit/trackit-server/aws/usageReports/rds"
	"github.com/trackit/trackit-server/costs/tags"
	"github.com/trackit/trackit-server/es"
	ts3 "github.com/trackit/trackit-server/s3/costs"
	"github.com/trackit/trackit-server/usageReports/ec2"
	"github.com/trackit/trackit-server/usageReports/ri"
)

type (
	Reservations struct {
		// Reservations
		TotalActiveRI      int64   // #
		TotalActiveRICost  float64 // $ invested
		ReservesWillExpire int     // $ invested
	}

	LowUsedInstance struct {
		InstanceType       string
		ComputationalPower float64
		Cost               float64
		Names              []string
	}

	GeneralInformation struct {
		// Reservations
		Reservations Reservations

		// LowUsedInstances
		LowUsedInstancesEc2          int     // #
		TotalLowUsedInatancesEc2Cost float64 // $
		HistogramUnusedEc2           Histogram
		LowUsedInstancesRds          int // #

		// S3
		TotalUsageS3      float64 // GB
		TotalCostS3       float64 // $
		TotalInstancesS3  int     // #
		TotalDailyUsageS3 float64 // xB/day
		TotalDailyCostS3  float64 // $/day

		UsageProportion      float64 // %
		DiscountedProportion float64 // %

		// Ec2
		TotalCostInstancesEC2  float64      // $
		PowerProductEc2        []Ec2Product // %
		TotalPowerEc2          float64      // %
		TotalEc2Instances      int64        // #
		UnreservedEc2Instances float64      // #

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

	RIProduct struct {
		InstanceType       string
		Reserves           int
		ComputationalPower float64
		Dates              []string
	}

	UnreservedSuggestion struct {
		InstanceType   string
		Difference     float64
		RICost         float64
		Machines       int
		UnreservedCost float64
	}

	Ec2Product struct {
		Family string
		Value  float64
	}
)

func formatEmail(reportRI ri.ResponseReservedInstance, reservedInstances []tri.ReservedInstance, s3info ts3.BucketsInfo, tagsValues tags.ProductsTagsResponse, costReport es.SimplifiedCostsDocument, ec2Instances []ec2.InstanceReport, rdsInstances []rds.InstanceReport, productsPrice product.EC2ProductsPrice, begin time.Time, end time.Time) (string, error) {
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
	unusedFormated := formatUnusedEc2Instances(ec2Instances, generalInformation)
	unusedRdsFormated := formatUnusedRdsInstances(rdsInstances, generalInformation)
	tagsFormated := formatProductsUsageInstances(tagsValues, generalInformation)
	instancesFormated := formatUnreservedEc2(reportRI, productsPrice, begin, end)

	formatEc2InstancesProportion(ec2Instances, reservedInstances, generalInformation)
	proportionReserves(reportRI, generalInformation)

	generalInformation.HistogramUnusedEc2 = calculateHistogram(ec2Instances, 5)

	generalFormated := formatGeneral(generalInformation, begin, end)

	//email += begin.Format("2006-01-02")

	//email += "</h1></center>"

	email += generalFormated

	email += "<br /><hr /><br /><br /><h1>More information</h1><br /><br />"

	email += reservedFormated

	email += "<h2>Low Used Resources</h2><p>To calculate the low used resources, this report consider the <b>Average CPU</b> weekly usage lower than <b>10%</b> and the instance (RDS or EC2) with a peak of CPU usage lower than <b>60%</b>.</p><br />"
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
	formated.WriteString("<td>" + strconv.FormatInt(generalInformation.Reservations.TotalActiveRI, 10) + "</td></tr>")
	formated.WriteString("<td><b>Expiring EC2 Reserved Instances on next week<sup>1</sup></b></td>")
	formated.WriteString("<td>" + strconv.Itoa(generalInformation.Reservations.ReservesWillExpire) + "</td></tr>")
	formated.WriteString("<tr><td><b>Active EC2 Reserved Instances Cost</b></td>")
	formated.WriteString("<td>$ " + fToS(generalInformation.Reservations.TotalActiveRICost) + "</td></tr>")

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

	formated.WriteString("<tr><td><b>Histogram of percentages of <br />the weekly average of CPU usage</b></td>")
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
	formated.WriteString("<td>" + strconv.Itoa(generalInformation.LowUsedInstancesRds) + "</td></tr>")
	formated.WriteString("<tr><td><b>RDS Low Used Instances Cost</b></td>")
	formated.WriteString("<td>$ " + fToS(generalInformation.LowUsedRdsCost) + "</td></tr>")
	formated.WriteString("</table><br />")

	formated.WriteString("<sup>1</sup> The total number of machines that will expire in the next week.<br />")
	formated.WriteString("<sup>2</sup> Only instances that generate some cost report in the bill of this week is considered in this count, so it doens't overleap the number of active reserved instances.<br />")
	formated.WriteString("<sup>3</sup> The proportion of computational power calculate the normalized usage of different instances types in hours during this interval of report.<br />")
	formated.WriteString("<sup>4</sup> The distribution of instances by family regarding the sum of compute units (ECU) of all instances.<br />")
	formated.WriteString("<sup>5</sup> Low used instances consider average CPU usage lower than 10% and no peaks of use over 60%.<br />")
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

	for i := 0; i < 7 && len(tagsValues) > i; i++ {
		item := tagsValues[i]

		formated.WriteString("<tr><td>" + strconv.Itoa(i+1) + "</td>")
		formated.WriteString("<td>" + item.Application + "</td>")
		formated.WriteString("<td>" + item.Owner + "</td>")
		formated.WriteString("<td>$ " + fToS(item.Ec2Cost) + "</td></tr>")
	}

	formated.WriteString("</table><a href=\"http://trackit-client.apps.topaz-analytics.com/app/tags\">Ver mais</a><br />")

	return formated.String()
}

func formatEc2InstancesProportion(instances []ec2.InstanceReport, reservedInstances []tri.ReservedInstance, generalInformation *GeneralInformation) {
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
	min := 100.0
	max := 0.0
	// search for minmax
	for _, instance := range instances {
		min = math.Min(min, instance.Instance.Stats.Cpu.Average)
		max = math.Max(max, instance.Instance.Stats.Cpu.Average)
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
