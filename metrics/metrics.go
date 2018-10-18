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

type DisplayGeneral map[string]interface{}

// MetricGlobal is intended to be used as an quick acccess way to increase and decrease simple values such as `in/outMessageCount` and `..Authorization`
type MetricMap struct {
	mp *hashmap.HashMap
}

// MetricGlobal is the global instance of the metric hashmap
var MetricGlobal = &MetricMap{mp: hashmap.New(64)}
var handler codec.JsonHandle
var h = &handler

func startAPI(port uint16) {
	http.HandleFunc("/metrics", metricToJSON)
	http.ListenAndServe(":"+strconv.Itoa(int(port)), nil)
}

// metricToJSON creates raw view of current data of MetricGlobal
func metricToJSON(w http.ResponseWriter, r *http.Request) {
	disMtr, err := MetricMapToGoMap(MetricGlobal.mp)
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

// IncrementAtomicUint64Key executes an atomic increment on the specified key of the map
func (mp *MetricMap) IncrementAtomicUint64Key(key string) {
	IncrementAtomicUint64KeyOf(mp.mp, key)
}

// IncrementAtomicUint64KeyOf executes an atomic increment on the specified key of any given map
func IncrementAtomicUint64KeyOf(hmp *hashmap.HashMap, key string) {
	var amt uint64
	curamt, _ := hmp.GetOrInsert(key, &amt)
	count := (curamt).(*uint64)
	atomic.AddUint64(count, 1)
}

// IncreaseAtomicUint64Key executes an atomic increase of diff on the specified key of the map
func (mp *MetricMap) IncreaseAtomicUint64Key(key string, diff uint64) {
	IncreaseAtomicUint64KeyOf(mp.mp, key, diff)
}

// IncreaseAtomicUint64KeyOf executes an atomic increase of diff on the specified key of any given map
func IncreaseAtomicUint64KeyOf(hmp *hashmap.HashMap, key string, diff uint64) {
	var amt uint64
	curamt, _ := hmp.GetOrInsert(key, &amt)
	count := (curamt).(*uint64)
	atomic.AddUint64(count, diff)
}

// GetSubMapOf returns the saved MetricMap of the given key of any given map. Creates one if none exists
func GetSubMapOf(hmp *hashmap.HashMap, key string) (mp *MetricMap) {
	var m MetricMap
	val, loaded := hmp.GetOrInsert(key, &m)
	mp = (val).(*MetricMap)
	if !loaded {
		mp.mp = hashmap.New(128)
	}
	hmp.Set(key, mp)
	return
}

// GetSubMap returns the saved MetricMap of the given key of the map. Creates one if none exists
func (mp *MetricMap) GetSubMap(key string) (dmp *MetricMap) {
	dmp = GetSubMapOf(mp.mp, key)
	return
}

// SendMsgCountHandler increments SendMessageCount
func SendMsgCountHandler() {
	IncrementAtomicUint64KeyOf(MetricGlobal.mp, "SendMessageCount")
}

// RecvMsgCountHandler increments RecvMesssageCount
func RecvMsgCountHandler() {
	IncrementAtomicUint64KeyOf(MetricGlobal.mp, "RecvMesssageCount")
}

// RecvMsgLenHandler increses RecvTrafficBytesTotal by the length of the received message
func RecvMsgLenHandler(len uint64) {
	IncreaseAtomicUint64KeyOf(MetricGlobal.mp, "RecvTrafficBytesTotal", len)
}

// SendMsgLenHandler increases SendTrafficBytesTotal by the length of the send message
func SendMsgLenHandler(len uint64) {
	IncreaseAtomicUint64KeyOf(MetricGlobal.mp, "SendTrafficBytesTotal", len)
}

// MetricMapToGoMap transform Hashmap to normal Go Map
func MetricMapToGoMap(mp *hashmap.HashMap) (disMtr DisplayGeneral, err error) {
	disMtr = make(map[string]interface{}, 32)
	for k := range mp.Iter() {
		if m, ok := (k.Value).(*hashmap.HashMap); ok {
			dmp, e := MetricMapToGoMap(m)
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
