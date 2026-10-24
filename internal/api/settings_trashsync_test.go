package api_test

import (
	"net/http"
	"testing"
)

// "Also tell me when nothing changed" is off until it is switched on, and the
// switch is kept.
func TestNotifyTrashSyncAlwaysSetting(t *testing.T) {
	_, base, admin := loginNewServer(t)
	get := func() any {
		return getJSON[map[string]any](t, admin, base+"/api/settings")["notifyTrashSyncAlways"]
	}
	if get() != false {
		t.Fatalf("default: %v", get())
	}
	putJSONStatus(t, admin, base+"/api/settings", map[string]any{"notifyTrashSyncAlways": true}, http.StatusOK)
	if get() != true {
		t.Fatalf("after switching on: %v", get())
	}
	putJSONStatus(t, admin, base+"/api/settings", map[string]any{"notifyTrashSyncAlways": false}, http.StatusOK)
	if get() != false {
		t.Fatalf("after switching off: %v", get())
	}
}
