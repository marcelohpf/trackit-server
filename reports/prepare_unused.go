package reports

import (
	"bytes"
	"sort"
	"strings"

	"github.com/trackit/trackit-server/aws/usageReports/rds"
	"github.com/trackit/trackit-server/usageReports/ec2"
	trds "github.com/trackit/trackit-server/usageReports/rds"
)

func formatUnusedEc2Instances(instances []ec2.InstanceReport, gi *GeneralInformation) string {
	ec2 := getLowUsedEc2(instances, gi)
	return formatEc2Table(ec2)
}

func getLowUsedEc2(instances []ec2.InstanceReport, gi *GeneralInformation) []LowUsedInstance {

	lowUsed := make(map[string]LowUsedInstance)

	for _, instance := range instances {
		if ec2.IsInstanceUnused(instance.Instance) {

			cost := 0.0
			for _, value := range instance.Instance.Costs {
				cost += value
			}

			gi.LowUsedInstancesEc2 += 1
			gi.TotalLowUsedInatancesEc2Cost += cost

			instanceType := instance.Instance.Type
			prev, _ := lowUsed[instanceType]

			prev.ComputationalPower += instance.Instance.NormalizationFactor
			prev.Cost += cost
			prev.Names = append(prev.Names, instance.Instance.Tags["Name"])

			lowUsed[instanceType] = prev
		}
	}

	var lowUsedinstances []LowUsedInstance
	for _, value := range lowUsed {
		lowUsedinstances = append(lowUsedinstances, value)
	}
	return lowUsedinstances
}

func formatEc2Table(lowUsed []LowUsedInstance) string {

	var formated bytes.Buffer

	formated.WriteString("<h4>EC2 Instances</h4>")
	formated.WriteString("<table width=\"600px\" cellspacing=\"0\" cellpadding=\"5\"><tr><th width=\"100px\" style=\"border-bottom: 1px solid; text-align: left;\">Family</th><th width=\"100px\" style=\"border-bottom: 1px solid; text-align: left;\">Cost</th><th style=\"border-bottom: 1px solid; text-align: left;\">Names</th></tr>")

	sort.Slice(lowUsed, func(i, j int) bool {
		return lowUsed[i].Cost > lowUsed[j].Cost
	})

	for i := 0; i < 5 && len(lowUsed) > i; i++ {
		formated.WriteString("<tr><td style=\"border-bottom: 1px dashed;\"><b>" + lowUsed[i].InstanceType + "</b></td>")
		formated.WriteString("<td style=\"border-bottom: 1px dashed;\">" + fToS(lowUsed[i].Cost) + "</td>")
		formated.WriteString("<td style=\"border-bottom: 1px dashed;\">" + strings.Join(lowUsed[i].Names, "<br />") + "</td></tr>")
	}

	formated.WriteString("</table><br />")
	return formated.String()
}

func formatUnusedRdsInstances(instances []rds.InstanceReport, gi *GeneralInformation) string {
	rds := getLowUsedRds(instances, gi)
	return formatEc2Table(rds)
}

func getLowUsedRds(instances []rds.InstanceReport, gi *GeneralInformation) []LowUsedInstance {

	lowUsed := make(map[string]LowUsedInstance)

	for _, instance := range instances {
		gi.TotalInstancesRds += 1

		if trds.IsInstanceUnused(instance.Instance) {
			gi.LowUsedInstancesRds += 1
			cost := 0.0
			for _, value := range instance.Instance.Costs {
				cost += value
			}
			instanceType := instance.Instance.DBInstanceClass

			prev, _ := lowUsed[instanceType]
			prev.ComputationalPower += instance.Instance.NormalizationFactor
			prev.Cost += cost
			prev.Names = append(prev.Names, instance.Instance.DBInstanceClass)
			lowUsed[instanceType] = prev

			gi.LowUsedRdsCost += cost
		}
	}

	values := make([]LowUsedInstance, 0, len(lowUsed))
	for _, value := range lowUsed {
		values = append(values, value)
	}

	return values

}

func formatRdsTable(lowUsed []LowUsedInstance) string {
	sort.Slice(lowUsed, func(i, j int) bool {
		return lowUsed[i].Cost > lowUsed[j].Cost
	})

	var formated bytes.Buffer

	formated.WriteString("<br /><br /><br /><h4>RDS Instances</h4>")
	formated.WriteString("<table width=\"600px\" cellspacing=\"0\" cellpadding=\"5\">")
	formated.WriteString("<tr><th width=\"100px\" style=\"border-bottom: 1px solid; text-align: left;\">Instance Type</th><th width=\"100px\" style=\"border-bottom: 1px solid; text-align: left;\">Cost</th><th style=\"border-bottom: 1px solid; text-align: left;\">Names</th></tr>")

	for i := 0; i < 5 && len(lowUsed) > i; i++ {
		formated.WriteString("<tr><td style=\"border-bottom: 1px dashed;\"><b>" + lowUsed[i].InstanceType + "</b></td>")
		formated.WriteString("<td style=\"border-bottom: 1px dashed;\">" + fToS(lowUsed[i].Cost) + "</td>")
		formated.WriteString("<td style=\"border-bottom: 1px dashed;\">" + strings.Join(lowUsed[i].Names, "<br />") + "</td></tr>")
	}

	formated.WriteString("</table>")

	return formated.String()
}
