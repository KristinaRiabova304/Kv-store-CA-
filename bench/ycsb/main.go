// kvstore-ycsb - YCSB для мого сховища. Генератор навантаження, сценарії й вимірювання - з go-ycsb
// (реалізація YCSB на Go), а тут лише драйвер, який перетворює операції YCSB на запити до мого API.
// Окремий Go-модуль, щоб залежності go-ycsb не потрапили в основний проєкт.
//
//	kvstore-ycsb load -P workloads/workloada -p kvstore.url=http://10.20.0.182:8080 -threads 16
//	kvstore-ycsb run  -P workloads/workloada -p kvstore.url=http://10.20.0.182:8080 -threads 16
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/magiconair/properties"
	"github.com/pingcap/go-ycsb/pkg/client"
	"github.com/pingcap/go-ycsb/pkg/measurement"
	"github.com/pingcap/go-ycsb/pkg/prop"
	_ "github.com/pingcap/go-ycsb/pkg/workload" // реєструє стандартний workload "core"
	"github.com/pingcap/go-ycsb/pkg/ycsb"
)

type multi []string

func (m *multi) String() string     { return strings.Join(*m, ",") }
func (m *multi) Set(v string) error { *m = append(*m, v); return nil }

func main() {
	if len(os.Args) < 2 || (os.Args[1] != "load" && os.Args[1] != "run") {
		fmt.Fprintln(os.Stderr, "usage: kvstore-ycsb load|run -P <workload file> [-p name=value] [-threads N]")
		os.Exit(2)
	}
	command := os.Args[1]
	fs := flag.NewFlagSet(command, flag.ExitOnError)
	var files, values multi
	fs.Var(&files, "P", "workload property file, can repeat")
	fs.Var(&values, "p", "property name=value, can repeat")
	threads := fs.Int("threads", 1, "client threads")
	_ = fs.Parse(os.Args[2:])

	props := properties.NewProperties()
	if len(files) > 0 {
		props = properties.MustLoadFiles(files, properties.UTF8, false)
	}
	for _, v := range values {
		kv := strings.SplitN(v, "=", 2)
		if len(kv) != 2 {
			log.Fatalf("bad property %q, expected name=value", v)
		}
		props.Set(kv[0], kv[1])
	}
	// load лише записує recordcount записів, run виконує operationcount операцій сценарію
	props.Set(prop.DoTransactions, strconv.FormatBool(command == "run"))
	props.Set(prop.Command, command)
	props.Set(prop.ThreadCount, strconv.Itoa(*threads))

	measurement.InitMeasure(props)
	workload, err := ycsb.GetWorkloadCreator("core").Create(props)
	if err != nil {
		log.Fatalf("workload: %v", err)
	}
	db, err := kvCreator{}.Create(props)
	if err != nil {
		log.Fatalf("db: %v", err)
	}

	fmt.Println("***************** properties *****************")
	for k, v := range props.Map() {
		fmt.Printf("%q=%q\n", k, v)
	}
	fmt.Println("**********************************************")
	start := time.Now()
	client.NewClient(props, workload, client.DbWrapper{DB: db}).Run(context.Background())
	fmt.Printf("Run finished, takes %s\n", time.Since(start))
	measurement.Output()
	_ = db.Close()
}
