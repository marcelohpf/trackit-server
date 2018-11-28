package ri

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/trackit/trackit-server/db"
	"github.com/trackit/trackit-server/routes"
	"github.com/trackit/trackit-server/users"
)

type RiReportQueryParams struct {
	accountList []string
	begin       time.Time
	end         time.Time
	indexList   []string
}

type RiQueryParams struct {
	accountList []string
	begin       time.Time
	end         time.Time
	state       string
	indexList   []string
}

var riReportQueryArgs = []routes.QueryArg{
	routes.AwsAccountsOptionalQueryArg,
	routes.DateBeginQueryArg,
	routes.DateEndQueryArg,
}

var riQueryArgs = []routes.QueryArg{
	routes.AwsAccountsOptionalQueryArg,
	routes.DateBeginQueryArg,
	routes.DateEndQueryArg,
	routes.QueryArg{
		Name:        "state",
		Type:        routes.QueryArgString{},
		Description: "Select `active` and `retired` reservations. If you ommit this parameters, it will return both.",
		Optional:    true,
	},
}

func init() {
	routes.MethodMuxer{
		http.MethodGet: routes.H(getReservedInstances).With(
			db.RequestTransaction{Db: db.Db},
			users.RequireAuthenticatedUser{users.ViewerAsParent},
			routes.QueryArgs(riQueryArgs),
			routes.Documentation{
				Summary:     "get the list of EC2 reserved instances",
				Description: "Responds with the list of EC2 reserved instances based on the queryparams passed to it",
			},
		),
	}.H().Register("/ri")
	routes.MethodMuxer{
		http.MethodGet: routes.H(getReportDiscountInstances).With(
			db.RequestTransaction{Db: db.Db},
			users.RequireAuthenticatedUser{users.ViewerAsParent},
			routes.QueryArgs(riReportQueryArgs),
			routes.Documentation{
				Summary:     "get the list usage and discounted EC2 instances",
				Description: "Responds with the list of usage and  instances based on the queryparams passed to it",
			},
		),
	}.H().Register("/ri/discount")
}

func getReservedInstances(request *http.Request, a routes.Arguments) (int, interface{}) {
	user := a[users.AuthenticatedUser].(users.User)
	tx := a[db.Transaction].(*sql.Tx)
	parsedParams := RiQueryParams{
		accountList: []string{},
		begin:       a[routes.DateBeginQueryArg].(time.Time),
		end:         a[routes.DateEndQueryArg].(time.Time),
		state:       "all",
	}
	if a[routes.AwsAccountsOptionalQueryArg] != nil {
		parsedParams.accountList = a[routes.AwsAccountIdsOptionalQueryArg].([]string)
	}
	returnCode, report, err := GetEC2ReservedInstances(request.Context(), parsedParams, user, tx)
	if err != nil {
		return returnCode, err
	} else {
		return returnCode, report
	}
}

func getReportDiscountInstances(request *http.Request, a routes.Arguments) (int, interface{}) {
	user := a[users.AuthenticatedUser].(users.User)
	tx := a[db.Transaction].(*sql.Tx)
	parsedParams := RiReportQueryParams{
		accountList: []string{},
		begin:       a[routes.DateBeginQueryArg].(time.Time),
		end:         a[routes.DateEndQueryArg].(time.Time),
	}
	if a[routes.AwsAccountsOptionalQueryArg] != nil {
		parsedParams.accountList = a[routes.AwsAccountIdsOptionalQueryArg].([]string)
	}
	returnCode, report, err := GetRC2ReportReservedInstances(request.Context(), parsedParams, user, tx)
	if err != nil {
		return returnCode, err
	} else {
		return returnCode, report
	}
}

func GetRiReportParams(begin time.Time, end time.Time, accountList []string) RiReportQueryParams {
	return RiReportQueryParams{
		begin:       begin,
		end:         end,
		accountList: accountList,
	}
}

func GetRiParams(begin time.Time, end time.Time, accountList []string, state string) RiQueryParams {
	return RiQueryParams{
		begin:       begin,
		end:         end,
		accountList: accountList,
		state:       state,
	}
}
