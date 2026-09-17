package credentialproduct

import "testing"

func TestParseSerialize(t *testing.T) {
	stored, err := Serialize([]string{"NRML", "MIS", "MIS", "CNC"})
	if err != nil {
		t.Fatal(err)
	}
	if stored != "MIS,CNC,NRML" {
		t.Fatalf("got %q", stored)
	}
	got := Parse(stored)
	if len(got) != 3 || got[0] != "MIS" || got[1] != "CNC" || got[2] != "NRML" {
		t.Fatalf("parse: %#v", got)
	}
}

func TestNormalizeSettingsMultiGated(t *testing.T) {
	_, _, err := NormalizeSettings("NSE", []string{"MIS", "CNC"}, false)
	if err == nil {
		t.Fatal("expected plan error")
	}
	_, stored, err := NormalizeSettings("NSE", []string{"MIS", "CNC"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if stored != "MIS,CNC" {
		t.Fatalf("got %q", stored)
	}
}

func TestResolveOrderProduct(t *testing.T) {
	p, err := ResolveOrderProduct("MIS,CNC", "CNC")
	if err != nil || p != "CNC" {
		t.Fatalf("got %q %v", p, err)
	}
	_, err = ResolveOrderProduct("MIS,CNC", "")
	if err == nil {
		t.Fatal("expected required product error")
	}
	p, err = ResolveOrderProduct("MIS", "")
	if err != nil || p != "MIS" {
		t.Fatalf("got %q %v", p, err)
	}
}
