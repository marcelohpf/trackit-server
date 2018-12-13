package reports

import (
	"bytes"
	"sort"
	"strconv"

	ts3 "github.com/trackit/trackit-server/s3/costs"
)

func formatS3Buckets(buckets ts3.BucketsInfo, generalInformation *GeneralInformation) string {

	products := prepareS3Buckets(buckets, generalInformation)

	return formatS3Table(products)

}

func prepareS3Buckets(buckets ts3.BucketsInfo, generalInformation *GeneralInformation) []S3Product {

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

	generalInformation.TotalDailyUsageS3 = generalInformation.TotalUsageS3 / WEEKDAYS
	generalInformation.TotalDailyCostS3 = generalInformation.TotalCostS3 / WEEKDAYS

	return instanceBuckets
}

func formatS3Table(instanceBuckets []S3Product) string {

	sort.Slice(instanceBuckets, func(i, j int) bool {
		return instanceBuckets[i].total > instanceBuckets[j].total
	})

	var formated bytes.Buffer

	formated.WriteString("<h2>Expensive S3 Buckets</h2>")
	formated.WriteString("These are the top five more expensive buckets used in the time interval of this report.")
	formated.WriteString("<table width=\"600px\" cellspacing=\"0\" cellpadding=\"5\"><tr><td></td><td><b>Name</b></td><td><b>Total Cost</b></td><td><b>Value (GB/$)</b></td><td><b>Week Size</b></td></tr>")

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
