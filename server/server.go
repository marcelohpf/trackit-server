//   Copyright 2017 MSolution.IO
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

package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"time"

	"github.com/satori/go.uuid"
	"github.com/trackit/jsonlog"

	_ "github.com/trackit/trackit-server/aws"
	_ "github.com/trackit/trackit-server/aws/routes"
	_ "github.com/trackit/trackit-server/aws/s3"
	"github.com/trackit/trackit-server/config"
	_ "github.com/trackit/trackit-server/costs"
	_ "github.com/trackit/trackit-server/costs/anomalies"
	_ "github.com/trackit/trackit-server/costs/diff"
	_ "github.com/trackit/trackit-server/costs/tags"
	"github.com/trackit/trackit-server/periodic"
	_ "github.com/trackit/trackit-server/plugins"
	_ "github.com/trackit/trackit-server/reports"
	"github.com/trackit/trackit-server/routes"
	_ "github.com/trackit/trackit-server/s3/costs"
	_ "github.com/trackit/trackit-server/usageReports/ec2"
	_ "github.com/trackit/trackit-server/usageReports/rds"
	_ "github.com/trackit/trackit-server/usageReports/ri"
	_ "github.com/trackit/trackit-server/users"
	_ "github.com/trackit/trackit-server/users/shared_account"
)

var buildNumber string = "unknown-build"
var backendId = getBackendId()

func init() {
	jsonlog.DefaultLogger = jsonlog.DefaultLogger.WithLogLevel(jsonlog.LogLevelDebug)
}

var tasks = map[string]func(context.Context) error{
	"server":                  taskServer,
	"ingest":                  taskIngest,
	"ingest-due":              taskIngestDue,
	"process-account":         taskProcessAccount,
	"process-account-plugins": taskProcessAccountPlugins,
	"anomalies-detection":     taskAnomaliesDetection,
	"check-user-entitlement":  taskCheckEntitlement,
	"process-history":         taskProcessHistoryAccount,
	"process-month-history":   taskMonthHistoryAccount,
	"setup-master-account":    taskSetupMasterAccount,
}

// dockerHostnameRe matches the value of the HOSTNAME environment variable when
// generated by Docker from the container ID.
var dockerHostnameRe = regexp.MustCompile(`[0-9a-z]{12}`)

func main() {
	ctx := context.Background()
	logger := jsonlog.DefaultLogger
	logger.Info("Started.", struct {
		BackendId string `json:"backendId"`
	}{backendId})
	if task, ok := tasks[config.Task]; ok {
		task(ctx)
	} else {
		knownTasks := make([]string, 0, len(tasks))
		for k := range tasks {
			knownTasks = append(knownTasks, k)
		}
		logger.Error("Unknown task.", map[string]interface{}{
			"knownTasks": knownTasks,
			"chosen":     config.Task,
		})
	}
}

var sched periodic.Scheduler

func schedulePeriodicTasks() {
	sched.Register(taskIngest, 6*60*time.Minute, "ingest-update")
	sched.Register(taskIngestDue, 6*60*time.Minute, "ingest-due-updates")
	sched.Register(taskProcessHistoryAccount, 24*60*time.Minute, "process-history")
	//sched.Register(taskProcessAccount, 750*time.Minute, "process-account")
	sched.Register(taskMonthHistoryAccount, 7*24*time.Hour, "process-month-history")
	sched.Start()
}

func taskServer(ctx context.Context) error {
	logger := jsonlog.LoggerFromContextOrDefault(ctx)
	initializeHandlers()
	if err := taskSetupMasterAccount(ctx); err != nil {
		return err
	}
	if config.Periodics {
		schedulePeriodicTasks()
		logger.Info("Scheduled periodic tasks.", nil)
	}
	logger.Info(fmt.Sprintf("Listening on %s.", config.HttpAddress), nil)
	err := http.ListenAndServe(config.HttpAddress, nil)
	logger.Error("Server stopped.", err.Error())
	return err
}

// initializeHandlers sets the HTTP server up with handler functions.
func initializeHandlers() {
	globalDecorators := []routes.Decorator{
		routes.RequestId{},
		routes.RouteLog{},
		routes.BackendId{backendId},
		routes.ErrorBody{},
		//routes.PanicAsError{},
		routes.Cors{
			AllowCredentials: true,
			AllowHeaders:     []string{"Content-Type", "Accept", "Authorization"},
			AllowOrigin:      []string{"*"},
		},
	}
	logger := jsonlog.DefaultLogger
	routes.DocumentationHandler().Register("/docs")
	for _, rh := range routes.RegisteredHandlers {
		applyDecoratorsAndHandle(rh.Pattern, rh.Handler, globalDecorators)
		logger.Info(fmt.Sprintf("Registered route %s.", rh.Pattern), nil)
	}
}

// applyDecoratorsAndHandle applies a list of decorators to a handler and
// registers it.
func applyDecoratorsAndHandle(p string, h routes.Handler, ds []routes.Decorator) {
	h = h.With(ds...)
	http.Handle(p, h)
}

// getBackendId returns an ID unique to the current process. It can also be set
// in the config to a determined string. It contains the build number.
func getBackendId() string {
	if config.BackendId != "" {
		return config.BackendId
	} else if hostname := os.Getenv("HOSTNAME"); dockerHostnameRe.Match([]byte(hostname)) {
		return fmt.Sprintf("%s-%s", hostname, buildNumber)
	} else {
		return fmt.Sprintf("%s-%s", uuid.NewV1().String(), buildNumber)
	}
}
