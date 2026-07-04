package storage

import "testing"

func TestDeepMerge(t *testing.T) {
	dst := map[string]any{
		"battery:0": map[string]any{"charge": "64", "voltage": "54000"},
		"vehicle":   map[string]any{"state": "stand-by"},
	}
	src := map[string]any{
		"battery:0": map[string]any{"charge": "65"},           // update nested leaf
		"gps":       map[string]any{"latitude": "52.5"},        // new nested map
	}
	deepMerge(dst, src)

	b0 := dst["battery:0"].(map[string]any)
	if b0["charge"] != "65" {
		t.Errorf("charge = %v, want 65", b0["charge"])
	}
	if b0["voltage"] != "54000" {
		t.Errorf("voltage should be preserved, got %v", b0["voltage"])
	}
	if dst["vehicle"].(map[string]any)["state"] != "stand-by" {
		t.Errorf("untouched key changed")
	}
	if dst["gps"].(map[string]any)["latitude"] != "52.5" {
		t.Errorf("new nested map not merged")
	}
}

func TestDeletePath(t *testing.T) {
	m := map[string]any{
		"gps": map[string]any{"latitude": "52.5", "longitude": "13.4"},
	}
	deletePath(m, "gps.latitude")
	gps := m["gps"].(map[string]any)
	if _, ok := gps["latitude"]; ok {
		t.Errorf("latitude was not deleted")
	}
	if gps["longitude"] != "13.4" {
		t.Errorf("sibling key removed")
	}

	// Missing intermediate path is a no-op (must not panic).
	deletePath(m, "nonexistent.child")
	deletePath(m, "gps.longitude.deep")
}
