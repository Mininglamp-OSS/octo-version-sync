package internal

import "time"

// beijingTZ 固定使用东八区，输出时间戳不带时区偏移。
var beijingTZ = time.FixedZone("Asia/Shanghai", 8*3600)

// beijingLayout 北京时间墙钟格式，不带 +08:00 后缀。
const beijingLayout = "2006-01-02T15:04:05"

// nowBeijing returns current Asia/Shanghai time in layout 2006-01-02T15:04:05.
func nowBeijing() string {
	return time.Now().In(beijingTZ).Format(beijingLayout)
}
