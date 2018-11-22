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
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"regexp"
	"sync"
	"time"

	"github.com/trackit/jsonlog"

	taws "github.com/trackit/trackit-server/aws"
	"github.com/trackit/trackit-server/aws/usageReports"
	"github.com/trackit/trackit-server/es"
)

const MonitorInstanceStsSessionName = "monitor-instance"

type (
	// InstanceReport is saved in ES to have all the information of an EC2 instance
	InstanceReport struct {
		Account    string    `json:"account"`
		ReportDate time.Time `json:"reportDate"`
		ReportType string    `json:"reportType"`
		Instance   Instance  `json:"instance"`
	}

	// Instance contains the information of an EC2 instance
	Instance struct {
		Id                  string             `json:"id"`
		Region              string             `json:"region"`
		State               string             `json:"state"`
		Purchasing          string             `json:"purchasing"`
		KeyPair             string             `json:"keyPair"`
		Type                string             `json:"type"`
		Tags                []Tag              `json:"tags"`
		Costs               map[string]float64 `json:"costs"`
		Stats               Stats              `json:"stats"`
		NormalizationFactor float64            `json:"normalizationFactor"`
		Family              string             `json:"family"`
	}

	// Stats contains statistics of an instance get on CloudWatch
	Stats struct {
		Cpu     Cpu      `json:"cpu"`
		Network Network  `json:"network"`
		Volumes []Volume `json:"volumes"`
	}

	// Cpu contains cpu statistics of an instance
	Cpu struct {
		Average float64 `json:"average"`
		Peak    float64 `json:"peak"`
	}

	// Network contains network statistics of an instance
	Network struct {
		In  float64 `json:"in"`
		Out float64 `json:"out"`
	}

	// Volume contains information about an EBS volume
	Volume struct {
		Id    string  `json:"id"`
		Read  float64 `json:"read"`
		Write float64 `json:"write"`
	}

	// Tag contains the key of a tag and his value
	Tag struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
)

// importInstancesToEs imports EC2 instances in ElasticSearch.
// It calls createIndexEs if the index doesn't exist.
func importInstancesToEs(ctx context.Context, aa taws.AwsAccount, instances []InstanceReport) error {
	logger := jsonlog.LoggerFromContextOrDefault(ctx)
	logger.Info("Updating EC2 instances for AWS account.", map[string]interface{}{
		"awsAccount": aa,
	})
	index := es.IndexNameForUserId(aa.UserId, IndexPrefixEC2Report)
	bp, err := utils.GetBulkProcessor(ctx)
	if err != nil {
		logger.Error("Failed to get bulk processor.", err.Error())
		return err
	}
	for _, instance := range instances {
		id, err := generateId(instance)
		if err != nil {
			logger.Error("Error when marshaling instance var", err.Error())
			return err
		}
		bp = utils.AddDocToBulkProcessor(bp, instance, TypeEC2Report, index, id)
	}
	bp.Flush()
	err = bp.Close()
	if err != nil {
		logger.Error("Fail to put EC2 instances in ES", err.Error())
		return err
	}
	logger.Info("EC2 instances put in ES", nil)
	return nil
}

func generateId(instance InstanceReport) (string, error) {
	ji, err := json.Marshal(struct {
		Account    string    `json:"account"`
		ReportDate time.Time `json:"reportDate"`
		Id         string    `json:"id"`
	}{
		instance.Account,
		instance.ReportDate,
		instance.Instance.Id,
	})
	if err != nil {
		return "", err
	}
	hash := md5.Sum(ji)
	hash64 := base64.URLEncoding.EncodeToString(hash[:])
	return hash64, nil
}

// merge function from https://blog.golang.org/pipelines#TOC_4
// It allows to merge many chans to one.
func merge(cs ...<-chan Instance) <-chan Instance {
	var wg sync.WaitGroup
	out := make(chan Instance)

	// Start an output goroutine for each input channel in cs. The output
	// copies values from c to out until c is closed, then calls wg.Done.
	output := func(c <-chan Instance) {
		for n := range c {
			out <- n
		}
		wg.Done()
	}
	wg.Add(len(cs))
	for _, c := range cs {
		go output(c)
	}

	// Start a goroutine to close out once all the output goroutines are
	// done. This must start after the wg.Add call.
	go func() {
		wg.Wait()
		close(out)
	}()
	return out
}

// normalizeInstanceType function to help normalize the compational factor of a
// instance as https://docs.aws.amazon.com/awsaccountbilling/latest/aboutv2/enhanced-lineitem-columns.html
func familyNormalizeFactor(instanceType string) (string, float64) {
	normalizationFactor := map[string]float64{
		"nano":     0.25,
		"micro":    0.5,
		"small":    1,
		"medium":   2,
		"large":    4,
		"xlarge":   8,
		"2xlarge":  16,
		"4xlarge":  32,
		"8xlarge":  64,
		"9xlarge":  72,
		"10xlarge": 80,
		"12xlarge": 96,
		"16xlarge": 128,
		"18xlarge": 144,
		"24xlarge": 192,
		"32xlarge": 256,
	}
	regex, _ := regexp.Compile("(.+)\\.([0-9]*x?[a-z]+$)")
	matchs := regex.FindStringSubmatch(instanceType)
	if len(matchs) == 3 {
		return matchs[1], normalizationFactor[matchs[2]]
	} else {
		return "unknown", 0
	}
}
