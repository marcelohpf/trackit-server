package ri

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/trackit/trackit-server/db"
	"github.com/trackit/trackit-server/routes"
	"github.com/trackit/trackit-server/users"
)

var riQueryArgs = []routes.QueryArg{
	routes.AwsAccountsOptionalQueryArg,
	routes.DateBeginQueryArg,
	routes.DateEndQueryArg,
	routes.QueryArg{
		Name:        "state",
		Type:        routes.QueryArgBool{},
		Description: "Select active and retired reservations. If you ommit this parameters, it will return both.",
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
}

func getReservedInstances(request *http.Request, a routes.Arguments) (int, interface{}) {
	user := a[users.AuthenticatedUser].(users.User)
	tx := a[db.Transaction].(*sql.Tx)
	parsedParams := RiQueryParams{
		accountList: []string{},
		begin:       a[routes.DateBeginQueryArg].(time.Time),
		end:         a[routes.DateEndQueryArg].(time.Time),
		state:       false,
	}
	if a[routes.AwsAccountsOptionalQueryArg] != nil {
		parsedParams.accountList = a[routes.AwsAccountIdsOptionalQueryArg].([]string)
	}
	if a[riQueryArgs[3]] != nil {
		parsedParams.state = a[riQueryArgs[3]].(bool)
	}
	returnCode, report, err := GetEC2ReservedInstances(request.Context(), parsedParams, user, tx)
	if err != nil {
		return returnCode, err
	} else {
		return returnCode, report
	}
}
