package reports

import (
	"bytes"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/trackit/trackit-server/aws"
	"github.com/trackit/trackit-server/aws/product"
	tri "github.com/trackit/trackit-server/aws/ri"
	"github.com/trackit/trackit-server/usageReports/ri"
)

func formatReserved(ris []tri.ReservedInstance, gi *GeneralInformation, startDate, endDate time.Time) string {

	nextReport := time.Date(endDate.Year(), endDate.Month()+2, 0, 23, 59, 59, 999999999, time.Local)

	reservations := riResume(ris, nextReport)
	gi.Reservations = reservations

	riProducts := getRIProducts(ris, nextReport)

	return formatTable(riProducts, nextReport)

}

func riResume(reservedInstances []tri.ReservedInstance, nextReport time.Time) Reservations {
	r := Reservations{}
	for _, instance := range reservedInstances {

		r.TotalActiveRICost += (float64(instance.InstanceCount) * instance.FixedPrice)
		r.TotalActiveRI += instance.InstanceCount
		if instance.EndDate.Before(nextReport) {
			r.ReservesWillExpire += int(instance.InstanceCount)
		}
	}
	return r
}

func getRIProducts(reservedInstances []tri.ReservedInstance, nextReport time.Time) []RIProduct {

	//computational := make(map[string]float64)

	expireReserve := make(map[string]int)
	expireReserveComputational := make(map[string]float64)
	expireReserveDate := make(map[string]map[string]int)

	for _, instance := range reservedInstances {

		if instance.EndDate.Before(nextReport) {

			expireReserve[instance.InstanceType] += int(instance.InstanceCount)
			expireReserveComputational[instance.InstanceType] += (float64(instance.InstanceCount) * instance.NormalizationFactor)

			if len(expireReserveDate[instance.InstanceType]) == 0 {
				expireReserveDate[instance.InstanceType] = make(map[string]int)
			}

			expireReserveDate[instance.InstanceType][instance.EndDate.Format("2006-01-02")] += 1
		}
	}

	var reserves []RIProduct
	for instanceType, value := range expireReserve {
		dates := collectOrderedDates(expireReserveDate[instanceType])

		reserves = append(reserves, RIProduct{
			InstanceType:       instanceType,
			Reserves:           value,
			ComputationalPower: expireReserveComputational[instanceType],
			Dates:              dates,
		})
	}

	return reserves

}

func formatTable(products []RIProduct, nextReport time.Time) string {

	if len(products) == 0 {
		return "<h2>No EC2 Reserved instances will expire until <b>" + nextReport.Format("2006-01-02") + "</b>.</h2>"
	}

	var formated bytes.Buffer
	formated.WriteString("<h2>Expirations of EC2 Reserved Instances</h2><br />The table below present only instances will expire until <b>" + nextReport.Format("2006-01-02") + "</b><br /> <br />")
	formated.WriteString("<table cellspacing=\"0\" cellpadding=\"5\"><tr><td width=\"100px\"><b>Instance Type</b></td><td width=\"200px\"><b>Expiring Instances</b></td><td width=\"100px\"><b>Date Expiration</b></td><td width=\"200px\"><b>Expiring Computational Power</b></td></tr>")

	for _, ri := range products {
		formated.WriteString("<tr><td>" + ri.InstanceType + "</td><td>" + strconv.Itoa(ri.Reserves) + "</td><td>" + strings.Join(ri.Dates, "<br />") + "</td><td>" + fToS(ri.ComputationalPower) + "</td></tr>")
	}
	formated.WriteString("</table><br /><a href=\"http://trackit-client.apps.topaz-analytics.com/app/reserves\">Ver mais</a><br />")

	return formated.String()
}

func collectOrderedDates(mapset map[string]int) []string {
	// colect sorted keys
	var keys []string
	for key := range mapset {
		keys = append(keys, key)
	}
	sort.Sort(sort.StringSlice(keys))
	return keys
}

func proportionReserves(reportRI ri.ResponseReservedInstance, gi *GeneralInformation) {
	totalUsages := make(map[string]float64)
	for key, report := range reportRI {
		for _, usage := range report {
			totalUsages[key] += usage.NormalizationFactor
		}
	}

	total := totalUsages["Usage"] + totalUsages["DiscountedUsage"]
	if total > 0 {
		gi.UsageProportion = (totalUsages["Usage"] / total) * 100
		gi.DiscountedProportion = (totalUsages["DiscountedUsage"] / total) * 100
	}
}

func formatUnreservedEc2(rRI ri.ResponseReservedInstance, ec2pp product.EC2ProductsPrice, startDate, endDate time.Time) string {
	usages := rRI["Usage"]
	suggestions := getUnreservedInstance(usages, ec2pp, startDate, endDate)
	submitUnreservedEc2(suggestions)
	return formatTableUnreservedEc2(suggestions)
}

func formatTableUnreservedEc2(us []UnreservedSuggestion) string {

	sort.Slice(us, func(i, j int) bool {
		return us[i].UnreservedCost > us[j].UnreservedCost
	})

	var formated bytes.Buffer

	formated.WriteString("<h2>On Demand EC2 Instances that can be reserved</h2>")
	formated.WriteString("These machines are <b>On Demand</b> usage and can be replaced by Reserved Instances.<br /><br /><br />")
	formated.WriteString("<table cellspacing=\"0\" cellpadding=\"5\">")
	formated.WriteString("<tr><td></td><td width=\"120px\"><b>Instance Type</b></td><td><b>Machines <sup>1</sup></b></td><td width=\"200px\"><b>Cost On Demand</b></td><td><b>Reservation Could Reduce Cost To <sup>2</sup></b></td><td><b>Difference of</b></td></tr>")

	suggestionsCount := 0

	for i := 0; suggestionsCount < 7 && len(us) > i; i++ {
		usage := us[i]

		if usage.Difference > 0 {
			suggestionsCount++
			formated.WriteString("<tr><td>" + strconv.Itoa(suggestionsCount) + "</td><td>" + usage.InstanceType + "</td>")
			formated.WriteString("<td>" + strconv.Itoa(usage.Machines) + "</td>")
			formated.WriteString("<td>$ " + fToS(usage.UnreservedCost) + "</td>")
			formated.WriteString("<td>$ " + fToS(usage.RICost) + "</td>")
			formated.WriteString("<td>" + fToS(usage.Difference) + "%</td>")
			formated.WriteString("</tr>")
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

func getUnreservedInstance(usages []tri.ReservedInstanceReport, productsPrice product.EC2ProductsPrice, startDate, endDate time.Time) []UnreservedSuggestion {
	interval := endDate.Sub(startDate)

	var unreserveds []UnreservedSuggestion
	for _, usage := range usages {

		instanceType := usage.Family + "." + aws.InverseNormalizationFactor(usage.NormalizationFactor)

		if usage.Family != "" && productsPrice[instanceType] > 0 && interval.Hours() > 0 && usage.NormalizationFactor > 0 {

			hoursUsage := usage.NormalizedUsage / usage.NormalizationFactor
			machines := math.Ceil(hoursUsage / float64(interval.Hours()))

			riCost := machines * productsPrice[instanceType] * interval.Hours()
			difference := 100.0 * (usage.Cost - riCost) / usage.Cost
			unreserveds = append(unreserveds, UnreservedSuggestion{
				InstanceType:   instanceType,
				RICost:         riCost,
				Machines:       int(machines),
				Difference:     difference,
				UnreservedCost: usage.Cost,
			})
		}
	}
	return unreserveds
}
