package metrics

import (
	"net/http/httptest"
	"testing"
)

func TestServer(t *testing.T) {
	go startAPI(3453)
	mp := GetSubMapOf(MetricGlobal.mp, "tester")
	IncrementAtomicUint64KeyOf(mp.mp, "test")
	IncreaseAtomicUint64KeyOf(mp.mp, "test", 54)
	r := httptest.NewRequest("", "localhost:3444", nil)
	w := httptest.NewRecorder()
	metricToJSON(w, r)
}

func TestFlat(t *testing.T) {
	IncrementAtomicUint64KeyOf(MetricGlobal.mp, "testing")
	IncreaseAtomicUint64KeyOf(MetricGlobal.mp, "testing", 54)
}

func TestMap(t *testing.T) {
	mp := GetSubMapOf(MetricGlobal.mp, "tester")
	IncrementAtomicUint64KeyOf(mp.mp, "test")
	IncreaseAtomicUint64KeyOf(mp.mp, "test", 54)
}

func TestBuiltinHandlers(t *testing.T) {
	RecvMsgCountHandler()
	SendMsgCountHandler()
	SendMsgLenHandler(54)
	RecvMsgLenHandler(54)
}

func TestConvert(t *testing.T) {
	MetricGlobal.MetricMapToGoMap()
}
