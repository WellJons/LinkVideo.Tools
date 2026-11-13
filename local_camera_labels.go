package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

var cameraNamesOnce sync.Once
var cameraNames map[string]string

func cameraDisplayName(key string) string {
	cameraNamesOnce.Do(func() {
		cameraNames = map[string]string{}
		for _, root := range []string{os.Getenv("ProgramFiles(x86)"), os.Getenv("ProgramFiles")} {
			if root == "" {
				continue
			}
			b, e := os.ReadFile(filepath.Join(root, "LinkVideo.Server", "local.config"))
			if e != nil {
				continue
			}
			var c struct {
				Cameras []struct {
					ID   int64  `json:"id_camera"`
					Name string `json:"local_camera_name"`
				} `json:"cameras"`
			}
			if json.Unmarshal(b, &c) != nil {
				continue
			}
			for _, cam := range c.Cameras {
				name := strings.TrimSpace(cam.Name)
				if name != "" && cam.ID > 0 {
					cameraNames["linkvideo_"+strconv.FormatInt(cam.ID, 10)] = name
				}
			}
		}
	})
	if s := cameraNames[key]; s != "" {
		return s + " · " + key
	}
	return key
}
