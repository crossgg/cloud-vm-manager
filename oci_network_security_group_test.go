package main

import "testing"

func TestAppendUniqueString(t *testing.T) {
	values := appendUniqueString([]string{"nsg-1"}, "nsg-1")
	if len(values) != 1 {
		t.Fatalf("expected duplicate value to be ignored: %+v", values)
	}

	values = appendUniqueString(values, "nsg-2")
	if len(values) != 2 || values[1] != "nsg-2" {
		t.Fatalf("expected new value to be appended: %+v", values)
	}
}

func TestChunkStrings(t *testing.T) {
	chunks := chunkStrings([]string{"a", "b", "c"}, 2)
	if len(chunks) != 2 || len(chunks[0]) != 2 || len(chunks[1]) != 1 {
		t.Fatalf("unexpected chunks: %+v", chunks)
	}
}
