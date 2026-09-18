package domain

import "testing"

func TestCloudAccountIdentityIgnoresSessionRotation(t *testing.T) {
	a, b := DefaultConfig(), DefaultConfig()
	a.Pan115Cookie = "UID=123_A1_old; SEID=old"
	b.Pan115Cookie = "UID=123_A2_new; SEID=new"
	if CloudAccountKey(a, "115") != CloudAccountKey(b, "115") {
		t.Fatal("cookie rotation changed account")
	}
	b.Pan115Cookie = "UID=456_A1_old"
	if CloudAccountKey(a, "115") == CloudAccountKey(b, "115") {
		t.Fatal("different UID matched")
	}
	a.PikpakEmail = " User@Example.com "
	b.PikpakEmail = "user@example.com"
	b.PikpakPassword = "new"
	if CloudAccountKey(a, "pikpak") != CloudAccountKey(b, "pikpak") {
		t.Fatal("password rotation changed account")
	}
}
