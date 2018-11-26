package ri

import (
	"context"
	"encoding/json"
	"gopkg.in/olivere/elastic.v5"
	"net/http"

	"github.com/trackit/jsonlog"
	"github.com/trackit/trackit-server/aws/ri"
	"github.com/trackit/trackit-server/errors"
)

type (
	ResponseEsReservedInstance struct {
		UsageType struct {
			Buckets []struct {
				Key    string `json:"key"`
				Family struct {
					Buckets []struct {
						Key    string `json:"key"`
						Factor struct {
							Buckets []struct {
								Key   float64 `json:"key"`
								Usage struct {
									Value float64 `json:"value"`
								} `json:"usageAmount"`
							} `json:"buckets"`
						} `json:"factor"`
					} `json:"buckets"`
				} `json:"family"`
			} `json:"buckets"`
		} `json:"usage"`
	}

	ResponseReservedInstance map[string][]ri.ReservedInstanceReport
)

func prepareResponseRi(ctx context.Context, res *elastic.SearchResult) (int, []ri.ReservedInstance, error) {
	logger := jsonlog.LoggerFromContextOrDefault(ctx)
	var instances []ri.ReservedInstance

	if res.Hits.TotalHits == 0 {
		logger.Warning("Query not found any result.", nil)
		return http.StatusOK, nil, nil
	}

	for _, instance := range res.Hits.Hits {
		var response ri.ReservedInstance
		err := json.Unmarshal(*instance.Source, &response)

		if err != nil {
			logger.Error("Error while unmarshaling ES RI response", err)
			return http.StatusInternalServerError, nil, errors.GetErrorMessage(ctx, err)
		}
		instances = append(instances, response)
	}

	//err := json.Unmarshal(res.Hits.Hits.Source, &response)
	return http.StatusOK, instances, nil
}

func prepareResponseRiResult(ctx context.Context, res *elastic.SearchResult) (int, ResponseReservedInstance, error) {
	logger := jsonlog.LoggerFromContextOrDefault(ctx)
	var instancesReport ResponseEsReservedInstance

	err := json.Unmarshal(*res.Aggregations["usage"], &instancesReport.UsageType)

	if err != nil {
		logger.Error("Error while unmarshaling reserved instance report.", err)
		return http.StatusInternalServerError, nil, errors.GetErrorMessage(ctx, err)
	}

	response := ResponseReservedInstance{}

	for _, usage := range instancesReport.UsageType.Buckets {
		var instances []ri.ReservedInstanceReport
		for _, family := range usage.Family.Buckets {
			for _, usageFactor := range family.Factor.Buckets {
				instances = append(instances, ri.ReservedInstanceReport{
					Type:                usage.Key,
					Family:              family.Key,
					NormalizationFactor: usageFactor.Key,
					NormalizedUsage:     usageFactor.Usage.Value,
				})
			}
		}
		if instances != nil {
			response[usage.Key] = instances
		}
	}
	return http.StatusOK, response, nil
}
