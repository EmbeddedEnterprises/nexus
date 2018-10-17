package metrics

import (
	"net/http"
	"strconv"
	"sync/atomic"

	"github.com/cornelk/hashmap"
	"github.com/gammazero/nexus/stdlog"
	"github.com/ugorji/go/codec"
)

var (
	logger stdlog.StdLog
)

type displayGeneral map[string]interface{}

// MetricGlobal is intended to be used as an quick acccess way to increase and decrease simple values such as `in/outMessageCount` and `..Authorization`
type MetricMap struct {
	mp *hashmap.HashMap
}

// MetricGlobal is the global instance of the metric hashmap
var MetricGlobal = &MetricMap{mp: hashmap.New(64)}
var handler codec.JsonHandle
var h = &handler

// Init offers initialization for metric api
func Init(port uint16, expose bool, tls bool) {
	if expose {
		// go startAPI(port)
	}
}

func startAPI(port uint16) {
	http.HandleFunc("/metrics", metricToJSON)
	http.ListenAndServe(":"+strconv.Itoa(int(port)), nil)
}

// metricToJSON creates raw view of current data of MetricGlobal
func metricToJSON(w http.ResponseWriter, r *http.Request) {
	disMtr, err := processMtr(MetricGlobal.mp)
	if err != nil {
		return
	}
	buffer := make([]byte, 128)
	encoder := codec.NewEncoderBytes(&buffer, h)
	encoder.Encode(disMtr)
	if err != nil {
		return
	}
	w.Write(buffer)
}

func (mp *MetricMap) IncrementAtomicUint64Key(key string) {
	IncrementAtomicUint64KeyOf(mp.mp, key)
}

func IncrementAtomicUint64KeyOf(hmp *hashmap.HashMap, key string) {
	var amt uint64
	curamt, _ := hmp.GetOrInsert(key, &amt)
	count := (curamt).(*uint64)
	atomic.AddUint64(count, 1)
}

func (mp *MetricMap) IncreaseAtomicUint64Key(key string, diff uint64) {
	IncreaseAtomicUint64KeyOf(mp.mp, key, diff)
}

func IncreaseAtomicUint64KeyOf(hmp *hashmap.HashMap, key string, diff uint64) {
	var amt uint64
	curamt, _ := hmp.GetOrInsert(key, &amt)
	count := (curamt).(*uint64)
	atomic.AddUint64(count, diff)
}

func GetSubMapOf(hmp *hashmap.HashMap, key string) (mp *hashmap.HashMap) {
	var m hashmap.HashMap
	val, loaded := hmp.GetOrInsert(key, &m)
	mp = (val).(*hashmap.HashMap)
	if !loaded {
		mp = hashmap.New(128)
	}
	hmp.Set(key, mp)
	return
}

func (mp *MetricMap) GetSubMap(key string) (dmp *hashmap.HashMap) {
	dmp = GetSubMapOf(mp.mp, key)
	return
}

func SendMsgCountHandler() {
	IncrementAtomicUint64KeyOf(MetricGlobal.mp, "SendMessageCount")
}

func RecvMsgCountHandler() {
	IncrementAtomicUint64KeyOf(MetricGlobal.mp, "RecvMesssageCount")
}

func RecvMsgLenHandler(len uint64) {
	IncreaseAtomicUint64KeyOf(MetricGlobal.mp, "RecvTrafficBytesTotal", len)
}

func SendMsgLenHandler(len uint64) {
	IncreaseAtomicUint64KeyOf(MetricGlobal.mp, "SendTrafficBytesTotal", len)
}

func processMtr(mp *hashmap.HashMap) (disMtr displayGeneral, err error) {
	disMtr = make(map[string]interface{}, 32)
	for k := range mp.Iter() {
		if m, ok := (k.Value).(*hashmap.HashMap); ok {
			dmp, e := processMtr(m)
			if err != nil {
				err = e
				return
			}
			disMtr[(k.Key).(string)] = dmp
		} else {
			disMtr[(k.Key).(string)] = k.Value
		}
	}
	return
}
