package ri

import (
	"context"
	// "fmt"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/trackit/jsonlog"

	"github.com/trackit/trackit-server/aws/ri"
	"github.com/trackit/trackit-server/errors"
	"github.com/trackit/trackit-server/es"
	"github.com/trackit/trackit-server/users"
	"gopkg.in/olivere/elastic.v5"
)

type RiQueryParams struct {
	accountList []string
	begin       time.Time
	end         time.Time
	state       bool
	indexList   []string
}

const maxAggregationSize = 0x7FFFFFFF

func GetEC2ReservedInstances(ctx context.Context, parsedParams RiQueryParams, user users.User, tx *sql.Tx) (int, []ri.ReservedInstance, error) {
	accountsAndIndexes, returnCode, err := es.GetAccountsAndIndexes(parsedParams.accountList, user, tx, ri.IndexPrefixEC2Reserved)
	if err != nil {
		return returnCode, nil, err
	}
	parsedParams.indexList = accountsAndIndexes.Indexes
	returnCode, res, err := makeElasticSearchRiRequest(ctx, parsedParams)
	if err != nil {
		return returnCode, nil, err
	}
	returnCode, instances, err := prepareResponseRi(ctx, res)
	if err != nil {
		return returnCode, nil, err
	}
	return returnCode, instances, nil
}

// makeElasticSearchRiRequest format the query and do a ElasticSearch query. It
// return a http code for execution, and if a error occur, it is returned with a
// empty result and a HTTP 500 status code
func makeElasticSearchRiRequest(ctx context.Context, params RiQueryParams) (int, *elastic.SearchResult, error) {
	l := jsonlog.LoggerFromContextOrDefault(ctx)
	index := strings.Join(params.indexList, ",")
	search := getElasticSearchRiParams(
		params,
		es.Client,
		index,
	)
	res, err := search.Do(ctx)
	if err != nil {
		if elastic.IsNotFound(err) {
			l.Warning("Query execution failed, ES index does not exists: "+index, err)
			return http.StatusOK, nil, err
		}
		l.Error("Query execution failed: ", err.Error())
		return http.StatusInternalServerError, nil, err
	}
	return http.StatusOK, res, nil
}

// getelasticSearchRiParams format the search request to get all Reserved
// Instances that begins from a specific type and is active or not
func getElasticSearchRiParams(params RiQueryParams, client *elastic.Client, index string) *elastic.SearchService {
	query := elastic.NewBoolQuery()
	query = query.Filter(elastic.NewRangeQuery("startDate").From(params.begin).To(params.end))

	if params.state {
		query = query.Filter(elastic.NewTermsQuery("state", params.state))
	}

	search := client.Search().Index(index).Query(query)
	return search
}

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
