package domain

import "testing"

func TestPublicationVisibility(t *testing.T) {
	pub := Publication{ProjectID: "p1", RevisionID: "r1", Public: true, Path: "/acme/payments/v1"}
	if !pub.VisibleTo(Actor{Anonymous: true}) {
		t.Fatal("public publication should be visible to anonymous readers")
	}
}

func TestPrivateRevisionHiddenFromAnonymous(t *testing.T) {
	pub := Publication{ProjectID: "p1", RevisionID: "r1", Public: false}
	if pub.VisibleTo(Actor{Anonymous: true}) {
		t.Fatal("private publication should be hidden from anonymous readers")
	}
	if !pub.VisibleTo(Actor{UserID: "u1", ProjectIDs: []string{"p1"}}) {
		t.Fatal("project member should see private publication")
	}
}
