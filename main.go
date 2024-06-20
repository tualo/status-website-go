package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	// "io"
	"os"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
)

var ctx = context.Background()
var rdb *redis.Client

var region string

type ConnectionState struct {
	Version     uint16 `json:"version"`
	CipherSuite uint16 `json:"cipher_suite"`
	ServerName  string `json:"server_name"`
}

type PingResult struct {
	WorkflowID    int    `json:"workflow_id"`
	RegionID      int    `json:"region_id"`
	TimeStamp     int64  `json:"timestamp"`
	Microseconds  int    `json:"microseconds"`
	StatusCode    int    `json:"status_code"`
	Status        string `json:"status"`
	ContentLength int    `json:"contentlength"`
	Proto         string `json:"proto"`
	ProtoMajor    int    `json:"proto_major"`
	ProtoMinor    int    `json:"proto_minor"`

	/*
		Id           string  `json:"id"`
		Region       string  `json:"region"`



		TimeStamp    int64   `json:"timestamp"`
		Milliseconds float64 `json:"milliseconds"`
		StatusCode   int     `json:"status_code"`
		Status       string  `json:"status"`
		// Error           string          `json:"error"`
		ContentLength   int             `json:"contentlength"`
		Proto           string          `json:"proto"`
		ProtoMajor      int             `json:"proto_major"`
		ProtoMinor      int             `json:"proto_minor"`
		ConnectionState ConnectionState `json:"connection_state"`
	*/
}

type WorkFlowItem struct {
	Workflow   string `json:"workflow"`
	WorkflowID int    `json:"workflow_id"`
	RegionID   int    `json:"region_id"`
	URL        string `json:"url"`
	Steps      []struct {
		StepId   int    `json:"step_id"`
		Method   string `json:"method"`
		StepType string `json:"step_type"`
	} `json:"steps"`
}

//type WorkFlow []struct{ WorkFlowItem }

type Configuration []struct {
	Addr     string `json:"addr"`
	DB       int    `json:"DB"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func check(e error) {
	if e != nil {
		panic(e)
	}
}

func (i PingResult) MarshalBinary() (data []byte, err error) {
	bytes, err := json.Marshal(i) // edited - changed to i
	if err != nil {
		panic(err)
	}
	return bytes, err
}

func TryCatch(f func()) func() error {
	return func() (err error) {
		defer func() {
			if panicInfo := recover(); panicInfo != nil {
				err = fmt.Errorf("%v, %s", panicInfo, string(debug.Stack()))
				return
			}
		}()
		f() // calling the decorated function
		return err
	}
}

var addr string
var username string
var password string
var db int

func RedisNewClient() bool {
	rdb = redis.NewClient(&redis.Options{
		Dialer: func(ctx context.Context, network, addr string) (net.Conn, error) {
			conn, err := net.Dial(network, addr)
			if err == nil {
				go func() {
					time.Sleep(5 * time.Second)
					conn.Close()
				}()
			} else {
				log.Print(err)
			}
			return conn, err
		},
		Addr:     addr,
		Password: password,
		Username: username,
		DB:       db,
	})

	ping := func() bool {
		_, err := rdb.Ping(ctx).Result()
		if err != nil {
			log.Print("error")
			log.Print(err)
			return false
		} else {
			log.Print("Connected")
			return true
		}
	}
	if ping() {

		vals, err := rdb.Do(ctx, "ROLE").Result()

		if err != nil {
			log.Print("error")
			log.Print(err)
			return false
		}

		if vals.([]interface{})[0].(string) == "master" {
			log.Print("Connected to master")
			return true
		} else {
			log.Print(">>>> Connected to slave")
			return false
		}

	}
	return false
}

func getConfig() []byte {
	var dat []byte
	f, err := os.Open("config.json")
	if err != nil {
		log.Fatalf("unable to read file: %v", err)
		panic(err)
	}
	defer f.Close()
	buf := make([]byte, 1024)
	for {
		n, err := f.Read(buf)
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Println(err)
			continue
		}
		if n > 0 {
			fmt.Println(string(buf[:n]))
			dat = append(dat, buf[:n]...)
		}
	}
	return dat
}

func RedisConnect() {

	//	dat, err := os.ReadFile("config.json")
	dat := getConfig()
	json_string := string(dat)
	var connections Configuration
	json.Unmarshal([]byte(json_string), &connections)

	for connections_index, connections_item := range connections {
		addr = connections_item.Addr
		username = connections_item.Username
		password = connections_item.Password
		db = connections_item.DB
		if RedisNewClient() {
			log.Print("Connected to Redis >>>>> ", connections_index)
			return
		}

	}

	panic("No connection to Redis")
}

func StoreResult(key string, result PingResult) {

	var list []PingResult

	val, err := rdb.Get(ctx, key).Result()
	// fmt.Println("V: %+v", val)
	if val != "" {
		// fmt.Println("V: %+v", val)
		//  fmt.Printf("PingResult: %+v", pingResult)
		json.Unmarshal([]byte(val), &list)
	} else {
		fmt.Printf("V %s: %+v", key, err)
	}
	list = append(list, result)

	b, err := json.Marshal(list)
	if err != nil {
		fmt.Println("error:", err)
	}
	fmt.Println("A:")

	ttl := time.Hour * 72
	err = rdb.Set(ctx, key, b, ttl).Err()
	if err != nil {
		panic(err)
	}
}

var timeout = time.Duration(2 * time.Second)

func dialTimeout(network, addr string) (net.Conn, error) {
	return net.DialTimeout(network, addr, timeout)
}

func PingHtml(item WorkFlowItem, index int) {
	url := item.URL
	method := item.Steps[index].Method
	// step_type := item.Steps[index].StepType

	start := time.Now()

	transport := http.Transport{
		Dial: dialTimeout,
	}

	client := http.Client{
		Transport: &transport,
	}

	var resp *http.Response
	var err error
	// var body []byte

	if method == "GET" {
		resp, err = client.Get(url)
		log.Printf("X")
	} else if method == "POST" {
		resp, err = http.Post(url,
			"application/x-www-form-urlencoded",
			strings.NewReader("name=foo&value=bar"))
	}

	elapsed := time.Since(start)

	//resp.TLS.SignedCertificateTimestamps

	log.Printf("A Query took %s", elapsed)

	if resp != nil {
		pingResult := PingResult{
			WorkflowID:    item.WorkflowID,
			RegionID:      item.RegionID,
			TimeStamp:     time.Now().Unix(),
			Microseconds:  int(elapsed.Microseconds()),
			StatusCode:    resp.StatusCode,
			Status:        resp.Status,
			ContentLength: int(resp.ContentLength),
			Proto:         resp.Proto,
			ProtoMajor:    0,
			ProtoMinor:    0,
			/*
				Id:            id,
				Region:        region,
				TimeStamp:     time.Now().Unix(),
				Milliseconds:  elapsed.Seconds() * 1000.0, //+ elapsed.Nanoseconds()/1000000.0,
				StatusCode:    resp.StatusCode,
				Status:        resp.Status,
				ContentLength: int(resp.ContentLength),
				Proto:         resp.Proto,
				ProtoMajor:    resp.ProtoMajor,
				ProtoMinor:    resp.ProtoMinor,
			*/
		}

		/*
			if resp.TLS != nil {
				connectionState := ConnectionState{
					Version:     resp.TLS.Version,
					CipherSuite: resp.TLS.CipherSuite,
					ServerName:  resp.TLS.ServerName,
				}
				pingResult.ConnectionState = connectionState
			}*/

		log.Printf("B Query took %s", elapsed)

		if true {
			fmt.Printf("PingResult: %+v", pingResult)
			uuid := uuid.NewString()
			StoreResult("sample_"+uuid, pingResult)
		}
	} else {
		pingResultError := PingResult{
			WorkflowID:    item.WorkflowID,
			RegionID:      item.RegionID,
			TimeStamp:     time.Now().Unix(),
			Microseconds:  int(elapsed.Microseconds()),
			StatusCode:    0,
			Status:        err.Error(),
			ContentLength: -1,
			Proto:         "",
			ProtoMajor:    0,
			ProtoMinor:    0,

			/*
				Id:            id,
				Region:        region,
				TimeStamp:     time.Now().Unix(),
				Milliseconds:  elapsed.Seconds() * 1000.0, //+ elapsed.Nanoseconds()/1000000.0,
				StatusCode:    0,
				Status:        err.Error(),
				ContentLength: -1,
				Proto:         "",
				ProtoMajor:    0,
				ProtoMinor:    0,
			*/
		}
		uuid := uuid.NewString()
		StoreResult("sample_"+uuid, pingResultError)
	}
	log.Printf("Query took %s", elapsed)

	if err != nil {
		log.Printf("Error %s", err)
		//log.Fatalln(err)
	}
	/*
		body, err = io.ReadAll(resp.Body)
		if err != nil {
			log.Fatalln(err)
		}
		sb := string(body)
		if false {
			log.Print(sb)
		}
	*/

}

func getWorkFlow() []WorkFlowItem {
	var list []WorkFlowItem

	vals, err := rdb.Keys(ctx, "workflow_"+region+"_*").Result()
	if err != nil {
		fmt.Println("error:", err)
	}
	for itm := range vals {
		val, err := rdb.Get(ctx, vals[itm]).Result()
		if val != "" {
			var workflow WorkFlowItem
			json.Unmarshal([]byte(val), &workflow)
			list = append(list, workflow)
		} else {
			fmt.Printf("V %s: %+v", "workflows", err)
		}
	}

	return list
}

func main() {
	var index int
	region = os.Getenv("REGION")
	if region == "" {
		region = "unkown"
	}

	log.Print("REGION: ", region)

	RedisConnect()

	start := time.Now()

	/*
		dat, err := os.ReadFile("sample/test.json")

		json_string := string(dat)
		// fmt.Println(json_string)
		var test_process WorkFlow

		json.Unmarshal([]byte(json_string), &test_process)
		// fmt.Printf("test_process : %+v", test_process)
	*/
	test_process := getWorkFlow()
	for workflow_index, workflow_item := range test_process {
		index = workflow_index

		for step_index, step_item := range workflow_item.Steps {
			index = step_index

			fmt.Printf("Workflow url: %s\n", workflow_item.URL)
			fmt.Printf("Workflow name: %s\n", workflow_item.Workflow)
			fmt.Printf("Step step_type: %s\n", step_item.StepType)
			fmt.Printf("Step method: %s\n", step_item.Method)
			index = index + 1
			PingHtml(workflow_item, step_index)
		}
	}

	elapsed := time.Since(start)
	log.Printf("Query took %s", elapsed)

}
