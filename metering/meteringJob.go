package metering

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	meteringModel "github.com/tmax-cloud/hypercloud-api-server/metering/model"
	"github.com/tmax-cloud/hypercloud-api-server/util"
	db "github.com/tmax-cloud/hypercloud-api-server/util/dataFactory"
	"k8s.io/klog"
)

const (
	// DB_URI      = fmt.Sprintf("port=%d host=%s user=%s "+"password=%s dbname=%s sslmode=disable", db.PORT, db.HOSTNAME, db.DB_USER, db.DB_PASSWORD, db.DB_NAME)

	METERING_INSERT_QUERY = "insert into metering (id,namespace,cpu,memory,storage,gpu,public_ip,private_ip, traffic_in, traffic_out, metering_time, status) " +
		"values ($1,$2,trunc($3,2),$4, $5,trunc($6,2),$7,$8,$9,$10,$11,$12)"
	METERING_DELETE_QUERY = "truncate metering"

	METERING_HOUR_INSERT_QUERY = "insert into metering_hour values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)"
	METERING_HOUR_SELECT_QUERY = "SELECT namespace, TRUNC(CAST(SUM(cpu)/COUNT(*) as numeric) ,2) as cpu, " +
		"TRUNC(CAST(SUM(memory)/COUNT(*) as numeric) ,0) as memory, TRUNC(CAST(SUM(storage)/COUNT(*) as numeric) ,0) as storage, " +
		"TRUNC(CAST(SUM(gpu)/COUNT(*) as numeric) ,2) as gpu, SUM(public_ip)/COUNT(*) as public_ip, SUM(private_ip)/COUNT(*) as private_ip, " +
		"TRUNC(CAST(SUM(traffic_in)/COUNT(*) as numeric) ,0) as traffic_in, TRUNC(CAST(SUM(traffic_out)/COUNT(*) as numeric) ,0) as traffic_out, " +
		"DATE_TRUNC('hour', metering_time) as metering_time, status FROM metering GROUP BY DATE_TRUNC('hour', metering_time), namespace, status"
	METERING_HOUR_UPDATE_QUERY = "update metering_hour set status = 'Merged' where status = 'Success'"
	METERING_HOUR_DELETE_QUERY = "delete from metering_hour where status = 'Merged'"

	METERING_DAY_INSERT_QUERY = "insert into metering_day values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)"
	METERING_DAY_SELECT_QUERY = "SELECT namespace, TRUNC(CAST(SUM(cpu)/COUNT(*) as numeric) ,2) as cpu, " +
		"TRUNC(CAST(SUM(memory)/COUNT(*) as numeric) ,0) as memory, TRUNC(CAST(SUM(storage)/COUNT(*) as numeric) ,0) as storage, " +
		"TRUNC(CAST(SUM(gpu)/COUNT(*) as numeric) ,2) as gpu, SUM(public_ip)/COUNT(*) as public_ip, SUM(private_ip)/COUNT(*) as private_ip, " +
		"TRUNC(CAST(SUM(traffic_in)/COUNT(*) as numeric) ,0) as traffic_in, TRUNC(CAST(SUM(traffic_out)/COUNT(*) as numeric) ,0) as traffic_out, " +
		"DATE_TRUNC('day', metering_time) as metering_time, status FROM metering_hour WHERE status='Success' GROUP BY DATE_TRUNC('day', metering_time), namespace, status"
	METERING_DAY_UPDATE_QUERY = "update metering_day set status = 'Merged' where status = 'Success'"
	METERING_DAY_DELETE_QUERY = "delete from metering_day where status = 'Merged'"

	METERING_MONTH_INSERT_QUERY = "insert into metering_month values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)"
	METERING_MONTH_SELECT_QUERY = "SELECT namespace, TRUNC(CAST(SUM(cpu)/COUNT(*) as numeric) ,2) as cpu, " +
		"TRUNC(CAST(SUM(memory)/COUNT(*) as numeric) ,0) as memory, TRUNC(CAST(SUM(storage)/COUNT(*) as numeric) ,0) as storage, " +
		"TRUNC(CAST(SUM(gpu)/COUNT(*) as numeric) ,2) as gpu, SUM(public_ip)/COUNT(*) as public_ip, SUM(private_ip)/COUNT(*) as private_ip, " +
		"TRUNC(CAST(SUM(traffic_in)/COUNT(*) as numeric) ,0) as traffic_in, TRUNC(CAST(SUM(traffic_out)/COUNT(*) as numeric) ,0) as traffic_out, " +
		"DATE_TRUNC('month', metering_time) as metering_time, status FROM metering_day WHERE status='Success' GROUP BY DATE_TRUNC('month', metering_time), namespace, status"
	METERING_MONTH_UPDATE_QUERY = "update metering_month set status = 'Merged' where status = 'Success'"
	METERING_MONTH_DELETE_QUERY = "delete from metering_month where status = 'Merged'"

	METERING_YEAR_INSERT_QUERY = "insert into metering_year values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)"
	METERING_YEAR_SELECT_QUERY = "SELECT namespace, TRUNC(CAST(SUM(cpu)/COUNT(*) as numeric) ,2) as cpu, " +
		"TRUNC(CAST(SUM(memory)/COUNT(*) as numeric) ,0) as memory, TRUNC(CAST(SUM(storage)/COUNT(*) as numeric) ,0) as storage, " +
		"TRUNC(CAST(SUM(gpu)/COUNT(*) as numeric) ,2) as gpu, SUM(public_ip)/COUNT(*) as public_ip, SUM(private_ip)/COUNT(*) as private_ip, " +
		"TRUNC(CAST(SUM(traffic_in)/COUNT(*) as numeric) ,0) as traffic_in, TRUNC(CAST(SUM(traffic_out)/COUNT(*) as numeric) ,0) as traffic_out, " +
		"DATE_TRUNC('year', metering_time) as metering_time, status FROM metering_month WHERE status='Success' GROUP BY DATE_TRUNC('year', metering_time), namespace, status"

	PROMETHEUS_URI = "http://prometheus-k8s.monitoring:9090/api/v1/query"
	//PROMETHEUS_GET_CPU_QUERY         = "namespace:container_cpu_usage_seconds_total:sum_rate"
	//PROMETHEUS_GET_MEMORY_QUERY      = "namespace:container_memory_usage_bytes:sum"
	PROMETHEUS_GET_CPU_QUERY         = "sum(kube_pod_container_resource_requests{resource=\"cpu\"})by(namespace)"
	PROMETHEUS_GET_MEMORY_QUERY      = "sum(kube_pod_container_resource_requests{resource=\"memory\"})by(namespace)"
	PROMETHEUS_GET_STORAGE_QUERY     = "sum(kube_persistentvolumeclaim_resource_requests_storage_bytes)by(namespace)"
	PROMETHEUS_GET_PUBLIC_IP_QUERY   = "count(kube_service_spec_type{type=\"LoadBalancer\"})by(namespace)"
	PROMETHEUS_GET_TRAFFIC_IN_QUERY  = "sum(rate(container_network_receive_bytes_total[1m]))by(namespace)"
	PROMETHEUS_GET_TRAFFIC_OUT_QUERY = "sum(rate(container_network_transmit_bytes_total[1m]))by(namespace)"
	//PROMETHEUS_GET_TRAFFIC_IN_QUERY  = "sum(istio_request_bytes_sum)by(destination_service, namespace)"
	//PROMETHEUS_GET_TRAFFIC_OUT_QUERY = "sum(istio_response_bytes_sum)by(destination_service, namespace)"
	//PROMETHEUS_GET_GPU_QUERY = "sum(nvidia_gpu_memory_used_bytes)by(namespace)"
)

var t time.Time
var file *os.File
var err error

func MeteringJob() {

	fileName := "./logs/api-server-metering" + time.Now().Format("2006-01-02") + ".log"
	file, err = os.OpenFile(
		fileName,
		os.O_APPEND|os.O_WRONLY|os.O_CREATE,
		os.FileMode(0600),
	)
	defer file.Close()
	if err != nil {
		klog.Errorln("Cannot open metering log file error : ", err)
		return
	}

	t = time.Now()
	fmt.Fprintf(file,
		"============= Metering Time =============\n"+
			"Current Time 	: "+t.Format("2006-01-02 15:04:05")+"\n"+
			"minute of hour	: %d\n"+
			"hour of day 	: %d\n"+
			"day of month 	: %d\n"+
			"day of year 	: %d\n",
		t.Minute(), t.Hour(), t.Day(), t.YearDay())

	// Merge into upper table every fixed time
	if t.Minute() == 0 {
		insertMeteringHour()
	}
	if t.Hour() == 0 && t.Minute() == 0 {
		insertMeteringDay()
	}
	if t.Day() == 1 && t.Hour() == 0 && t.Minute() == 0 {
		insertMeteringMonth()
	}
	if t.YearDay() == 1 && t.Day() == 1 && t.Hour() == 0 && t.Minute() == 0 {
		insertMeteringYear()
	}

	// Get data from Prometheus
	meteringData := makeMeteringMap()

	fmt.Fprintf(file, "============= Metering Data =============\n")
	for key, value := range meteringData {
		fmt.Fprintf(file, "%-35s : %f\n", key+"/cpu", value.Cpu)
		fmt.Fprintf(file, "%-35s : %d\n", key+"/memory", value.Memory)
		fmt.Fprintf(file, "%-35s : %d\n", key+"/storage", value.Storage)
		fmt.Fprintf(file, "%-35s : %d\n", key+"/publicIp", value.PublicIp)
		fmt.Fprintf(file, "%-35s : %d\n", key+"/trafficIn", value.TrafficIn)
		fmt.Fprintf(file, "%-35s : %d\n", key+"/trafficOut", value.TrafficOut)
		fmt.Fprintf(file, "-----------------------------------------\n")
	}
	//Insert into metering (new data)
	insertMeteringData(meteringData)
}

func insertMeteringData(meteringData map[string]*meteringModel.Metering) {
	fmt.Fprintf(file,
		"Insert into METERING Start!!\n"+
			"Current Time	: "+t.Format("2006-01-02 15:04:00")+"\n")

	for key, data := range meteringData {
		_, err = db.Dbpool.Exec(context.TODO(), METERING_INSERT_QUERY,
			uuid.New(),
			key,
			data.Cpu,
			data.Memory,
			data.Storage,
			data.Gpu,
			data.PublicIp,
			data.PrivateIp,
			data.TrafficIn,
			data.TrafficOut,
			t.Format("2006-01-02 15:04:00"), "Success")

		if err != nil {
			fmt.Fprintf(file, "%v\n", err)
			break
		}
	}

	if err != nil {
		fmt.Fprintf(file, "Insert into METERING failed..\n")
	} else {
		fmt.Fprintf(file, "Insert into METERING Success!!\n")
	}
}

func makeMeteringMap() map[string]*meteringModel.Metering {
	var meteringData = make(map[string]*meteringModel.Metering)
	cpu := getMeteringData(PROMETHEUS_GET_CPU_QUERY)
	for _, metric := range cpu.Result {
		var keys []string
		for k := range meteringData {
			keys = append(keys, k)
		}
		if util.Contains(keys, metric.Metric["namespace"]) {
			meteringData[metric.Metric["namespace"]].Cpu, _ = strconv.ParseFloat(metric.Value[1].(string), 64)
		} else {
			metering := new(meteringModel.Metering)
			metering.Namespace = metric.Metric["namespace"]
			metering.Cpu, _ = strconv.ParseFloat(metric.Value[1].(string), 64)
			meteringData[metric.Metric["namespace"]] = metering
		}
	}

	memory := getMeteringData(PROMETHEUS_GET_MEMORY_QUERY)
	for _, metric := range memory.Result {
		var keys []string
		for k := range meteringData {
			keys = append(keys, k)
		}
		if util.Contains(keys, metric.Metric["namespace"]) {
			meteringData[metric.Metric["namespace"]].Memory, _ = strconv.ParseUint(metric.Value[1].(string), 10, 64)
		} else {
			metering := new(meteringModel.Metering)
			metering.Namespace = metric.Metric["namespace"]
			metering.Memory, _ = strconv.ParseUint(metric.Value[1].(string), 10, 64)
			meteringData[metric.Metric["namespace"]] = metering
		}
	}

	storage := getMeteringData(PROMETHEUS_GET_STORAGE_QUERY)
	for _, metric := range storage.Result {
		var keys []string
		for k := range meteringData {
			keys = append(keys, k)
		}
		if util.Contains(keys, metric.Metric["namespace"]) {
			meteringData[metric.Metric["namespace"]].Storage, _ = strconv.ParseUint(metric.Value[1].(string), 10, 64)
		} else {
			metering := new(meteringModel.Metering)
			metering.Namespace = metric.Metric["namespace"]
			metering.Storage, _ = strconv.ParseUint(metric.Value[1].(string), 10, 64)
			meteringData[metric.Metric["namespace"]] = metering
		}
	}

	publicIp := getMeteringData(PROMETHEUS_GET_PUBLIC_IP_QUERY)
	for _, metric := range publicIp.Result {
		var keys []string
		for k := range meteringData {
			keys = append(keys, k)
		}
		if util.Contains(keys, metric.Metric["namespace"]) {
			meteringData[metric.Metric["namespace"]].PublicIp, _ = strconv.ParseUint(metric.Value[1].(string), 10, 64)
		} else {
			metering := new(meteringModel.Metering)
			metering.Namespace = metric.Metric["namespace"]
			metering.PublicIp, _ = strconv.ParseUint(metric.Value[1].(string), 10, 64)
			meteringData[metric.Metric["namespace"]] = metering
		}
	}

	trafficIn := getMeteringData(PROMETHEUS_GET_TRAFFIC_IN_QUERY)
	for _, metric := range trafficIn.Result {
		var keys []string
		for k := range meteringData {
			keys = append(keys, k)
		}
		//if strings.Contains(metric.Metric["destination_service"], "."+metric.Metric["namespace"]+".") {
		if util.Contains(keys, metric.Metric["namespace"]) {
			meteringData[metric.Metric["namespace"]].TrafficIn, _ = strconv.ParseUint(metric.Value[1].(string), 10, 64)
		} else {
			metering := new(meteringModel.Metering)
			metering.Namespace = metric.Metric["namespace"]
			metering.TrafficIn, _ = strconv.ParseUint(metric.Value[1].(string), 10, 64)
			meteringData[metric.Metric["namespace"]] = metering
		}
	}

	trafficOut := getMeteringData(PROMETHEUS_GET_TRAFFIC_OUT_QUERY)
	for _, metric := range trafficOut.Result {
		var keys []string
		for k := range meteringData {
			keys = append(keys, k)
		}
		//if strings.Contains(metric.Metric["destination_service"], "."+metric.Metric["namespace"]+".") {
		if util.Contains(keys, metric.Metric["namespace"]) {
			meteringData[metric.Metric["namespace"]].TrafficOut, _ = strconv.ParseUint(metric.Value[1].(string), 10, 64)
		} else {
			metering := new(meteringModel.Metering)
			metering.Namespace = metric.Metric["namespace"]
			metering.TrafficOut, _ = strconv.ParseUint(metric.Value[1].(string), 10, 64)
			meteringData[metric.Metric["namespace"]] = metering
		}
	}

	return meteringData
}

func getMeteringData(query string) meteringModel.MetricDataList {
	var metricResponse meteringModel.MetricResponse
	// Make Request Object
	req, err := http.NewRequest("GET", PROMETHEUS_URI, nil)
	if err != nil {
		fmt.Fprintf(file, "%v\n", err)
		panic(err)
	}

	// Add QueryParameter
	q := req.URL.Query()
	q.Add("query", query)
	req.URL.RawQuery = q.Encode()

	// Request with Client Object
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(file, "Prometheus Connection Failed.\n")
		for i := 0; i < 10; i++ {
			time.Sleep(time.Second * 5)
			resp, err = client.Do(req)
			if err == nil {
				break
			}
		}
		if err != nil {
			fmt.Fprintf(file, "%v\n", err)
			panic(err)
		}
	}
	defer resp.Body.Close()

	// Result
	bytes, _ := ioutil.ReadAll(resp.Body)

	if err := json.Unmarshal(bytes, &metricResponse); err != nil {
		klog.Error(err)
	}
	return metricResponse.Data
}

func insertMeteringYear() {
	fmt.Fprintf(file,
		"Insert into METERING_YEAR Start!!\n"+
			"Current Time	: "+t.Format("2006-01-02 15:04:00")+"\n")

	rows, err := db.Dbpool.Query(context.TODO(), METERING_YEAR_SELECT_QUERY)
	defer rows.Close()
	if err != nil {
		fmt.Fprintf(file, "%v\n", err)
		return
	}

	var meteringData meteringModel.Metering
	var status string
	for rows.Next() {
		err := rows.Scan(
			//&meteringData.Id,
			&meteringData.Namespace,
			&meteringData.Cpu,
			&meteringData.Memory,
			&meteringData.Storage,
			&meteringData.Gpu,
			&meteringData.PublicIp,
			&meteringData.PrivateIp,
			&meteringData.TrafficIn,
			&meteringData.TrafficOut,
			&meteringData.MeteringTime,
			&status)
		if err != nil {
			fmt.Fprintf(file, "%v\n", err)
			return
		}

		_, err = db.Dbpool.Exec(context.TODO(), METERING_YEAR_INSERT_QUERY,
			uuid.New(),
			meteringData.Namespace,
			meteringData.Cpu,
			meteringData.Memory,
			meteringData.Storage,
			meteringData.Gpu,
			meteringData.PublicIp,
			meteringData.PrivateIp,
			meteringData.TrafficIn,
			meteringData.TrafficOut,
			meteringData.MeteringTime.AddDate(0, -(util.MonthToInt(meteringData.MeteringTime.Month())-1),
				-(meteringData.MeteringTime.Day()-1)).Format("2006-01-02 00:00:00"),
			//date_format(metering_time,'%Y-01-01 00:00:00'),
			status)
		if err != nil {
			fmt.Fprintf(file, "%v\n", err)
			return
		}
	}
	fmt.Fprintf(file,
		"Insert into METERING_YEAR Success!!\n"+
			"--------------------------------------\n"+
			"Update METERING_MONTH Past data to 'Merged' Start!!\n")
	_, err = db.Dbpool.Exec(context.TODO(), METERING_MONTH_UPDATE_QUERY)
	if err != nil {
		fmt.Fprintf(file, "%v\n", err)
		return
	}
	fmt.Fprintf(file, "Update METERING_MONTH Past data to 'Merged' Success!!\n")
	/*
		klog.Infoln("--------------------------------------")
		klog.Infoln("Delete METERING for past 1 year Start!!")
		_, err = db.Exec(METERING_MONTH_DELETE_QUERY)
		if err != nil {
			fmt.Fprintf(file, "%v\n", err)
			return
		}
		klog.Infoln("Delete METERING for past 1 year Success!!")
	*/
}

func insertMeteringMonth() {
	fmt.Fprintf(file,
		"Insert into METERING_MONTH Start!!\n"+
			"Current Time	: "+t.Format("2006-01-02 15:04:00")+"\n")

	rows, err := db.Dbpool.Query(context.TODO(), METERING_MONTH_SELECT_QUERY)
	defer rows.Close()

	if err != nil {
		fmt.Fprintf(file, "%v\n", err)
		return
	}

	var meteringData meteringModel.Metering
	var status string
	for rows.Next() {
		err := rows.Scan(
			//&meteringData.Id,
			&meteringData.Namespace,
			&meteringData.Cpu,
			&meteringData.Memory,
			&meteringData.Storage,
			&meteringData.Gpu,
			&meteringData.PublicIp,
			&meteringData.PrivateIp,
			&meteringData.TrafficIn,
			&meteringData.TrafficOut,
			&meteringData.MeteringTime,
			&status)
		if err != nil {
			fmt.Fprintf(file, "%v\n", err)
			return
		}

		_, err = db.Dbpool.Exec(context.TODO(), METERING_MONTH_INSERT_QUERY,
			uuid.New(),
			meteringData.Namespace,
			meteringData.Cpu,
			meteringData.Memory,
			meteringData.Storage,
			meteringData.Gpu,
			meteringData.PublicIp,
			meteringData.PrivateIp,
			meteringData.TrafficIn,
			meteringData.TrafficOut,
			meteringData.MeteringTime.AddDate(0, 0,
				-(meteringData.MeteringTime.Day()-1)).Format("2006-01-02 00:00:00"),
			//date_format(metering_time,'%Y-%m-01 00:00:00'),
			status)
		if err != nil {
			fmt.Fprintf(file, "%v\n", err)
			return
		}
	}
	fmt.Fprintf(file,
		"Insert into METERING_MONTH Success!!\n"+
			"--------------------------------------\n"+
			"Update METERING_DAY Past data to 'Merged' Start!!\n")
	_, err = db.Dbpool.Exec(context.TODO(), METERING_DAY_UPDATE_QUERY)
	if err != nil {
		fmt.Fprintf(file, "%v\n", err)
		return
	}
	fmt.Fprintf(file, "Update METERING_DAY Past data to 'Merged' Success!!\n")
	/*
		klog.Infoln("--------------------------------------")
		klog.Infoln("Delete METERING for past 1 month Start!!")
		_, err = db.Exec(METERING_DAY_DELETE_QUERY)
		if err != nil {
			fmt.Fprintf(file, "%v\n", err)
			return
		}
		klog.Infoln("Delete METERING for past 1 month Success!!")
	*/
}

func insertMeteringDay() {
	fmt.Fprintf(file,
		"Insert into METERING_DAY Start!!\n"+
			"Current Time	: "+t.Format("2006-01-02 15:04:00")+"\n")

	rows, err := db.Dbpool.Query(context.TODO(), METERING_DAY_SELECT_QUERY)
	defer rows.Close()

	if err != nil {
		fmt.Fprintf(file, "%v\n", err)
		return
	}

	var meteringData meteringModel.Metering
	var status string
	for rows.Next() {
		err := rows.Scan(
			//&meteringData.Id,
			&meteringData.Namespace,
			&meteringData.Cpu,
			&meteringData.Memory,
			&meteringData.Storage,
			&meteringData.Gpu,
			&meteringData.PublicIp,
			&meteringData.PrivateIp,
			&meteringData.TrafficIn,
			&meteringData.TrafficOut,
			&meteringData.MeteringTime,
			&status)
		if err != nil {
			fmt.Fprintf(file, "%v\n", err)
			return
		}

		_, err = db.Dbpool.Exec(context.TODO(), METERING_DAY_INSERT_QUERY,
			uuid.New(),
			meteringData.Namespace,
			meteringData.Cpu,
			meteringData.Memory,
			meteringData.Storage,
			meteringData.Gpu,
			meteringData.PublicIp,
			meteringData.PrivateIp,
			meteringData.TrafficIn,
			meteringData.TrafficOut,
			meteringData.MeteringTime.Format("2006-01-02 00:00:00"), //date_format(metering_time,'%Y-%m-%d 00:00:00')
			status)
		if err != nil {
			fmt.Fprintf(file, "%v\n", err)
			return
		}
	}
	fmt.Fprintf(file,
		"Insert into METERING_DAY Success!!\n"+
			"--------------------------------------\n"+
			"Update METERING_HOUR Past data to 'Merged' Start!!\n")
	_, err = db.Dbpool.Exec(context.TODO(), METERING_HOUR_UPDATE_QUERY)
	if err != nil {
		fmt.Fprintf(file, "%v\n", err)
		return
	}
	fmt.Fprintf(file, "Update METERING_HOUR Past data to 'Merged' Success!!\n")
	/*
		klog.Infoln("--------------------------------------")
		klog.Infoln("Delete METERING for past 1 day Start!!")
		_, err = db.Exec(METERING_HOUR_DELETE_QUERY)
		if err != nil {
			fmt.Fprintf(file, "%v\n", err)
			return
		}
		klog.Infoln("Delete METERING for past 1 day Success!!")
	*/
}

func insertMeteringHour() {
	fmt.Fprintf(file,
		"Insert into METERING_HOUR Start!!\n"+
			"Current Time	: "+t.Format("2006-01-02 15:04:00")+"\n")

	rows, err := db.Dbpool.Query(context.TODO(), METERING_HOUR_SELECT_QUERY)
	defer rows.Close()

	if err != nil {
		fmt.Fprintf(file, "%v\n", err)
		return
	}

	var meteringData meteringModel.Metering
	var status string
	for rows.Next() {
		err := rows.Scan(
			//&meteringData.Id,
			&meteringData.Namespace,
			&meteringData.Cpu,
			&meteringData.Memory,
			&meteringData.Storage,
			&meteringData.Gpu,
			&meteringData.PublicIp,
			&meteringData.PrivateIp,
			&meteringData.TrafficIn,
			&meteringData.TrafficOut,
			&meteringData.MeteringTime,
			&status)
		if err != nil {
			fmt.Fprintf(file, "%v\n", err)
			return
		}

		_, err = db.Dbpool.Exec(context.TODO(), METERING_HOUR_INSERT_QUERY,
			uuid.New(),
			meteringData.Namespace,
			meteringData.Cpu,
			meteringData.Memory,
			meteringData.Storage,
			meteringData.Gpu,
			meteringData.PublicIp,
			meteringData.PrivateIp,
			meteringData.TrafficIn,
			meteringData.TrafficOut,
			meteringData.MeteringTime.Format("2006-01-02 15:00:00"),
			status)
		if err != nil {
			fmt.Fprintf(file, "%v\n", err)
			return
		}
	}
	fmt.Fprintf(file,
		"Insert into METERING_HOUR Success!!\n"+
			"--------------------------------------\n"+
			"Update METERING Past data to 'Merged' Start!!\n")
	_, err = db.Dbpool.Exec(context.TODO(), METERING_DELETE_QUERY)
	if err != nil {
		fmt.Fprintf(file, "%v\n", err)
		return
	}
	fmt.Fprintf(file, "Update METERING Past data to 'Merged' Success!!\n")
}
