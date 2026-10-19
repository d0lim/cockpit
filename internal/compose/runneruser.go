package compose

import "fmt"

func RunnerUser(goos string, uid, gid int) string {
	if goos != "linux" || uid <= 0 || gid < 0 {
		return ""
	}
	return fmt.Sprintf("%d:%d", uid, gid)
}
