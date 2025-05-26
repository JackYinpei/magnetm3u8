package utils

import "testing"

func TestExtractPath(t *testing.T) {
	res := ExtractPath("http://43.156.74.32:7070/video/task_3/index.m3u8")
	if res != "task_3/index.m3u8" {
		t.Errorf("ExtractPath(\"http://43.156.74.32:7070/task_3/index.m3u8\") = %s; want \"task_3/index.m3u8\"", res)
	}
}
