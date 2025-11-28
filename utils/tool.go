package utils

import (
	crand "crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"math"
	"math/big"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"
)

type timeNumber interface {
	~int | ~int32 | ~int64 | ~uint | ~uint32 | ~uint64
}

type Number interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 | ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64
}

func Base64Encode(str string) string {
	return base64.StdEncoding.EncodeToString([]byte(strings.TrimSpace(strings.Trim(str, "\n"))))
}
func Base64Decode(str string) string {
	bstr, err := base64.StdEncoding.DecodeString(strings.TrimSpace(strings.Trim(str, "\n")))
	if err != nil {
		return str
	}
	return string(bstr)
}
func Hash(str string) string {
	return HashBytes([]byte(str))
}

func HashBytes(data []byte) string {
	b := sha256.Sum224(data)
	return hex.EncodeToString(b[:])
}

func TimeFormat[T timeNumber](t T, loc *time.Location) string {
	return time.Unix(int64(t), 0).In(loc).Format(time.DateTime)
}

// 四舍五入保留小数位
func NumberFormat[T ~float32 | ~float64](f T, n ...uint) float64 {
	num := uint(2)
	if len(n) > 0 {
		num = n[0]
	}
	nu := math.Pow(10, float64(num))
	return math.Round(float64(f)*nu) / nu
}

// 文件是否存在
func FileExist(path string) bool {
	_, err := os.Stat(path)
	return err == nil || os.IsExist(err)
}

// 创建目录
func Mkdir(path string) error {
	// 从路径中取目录
	dir := filepath.Dir(path)
	// 获取信息, 即判断是否存在目录
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		// 生成目录
		if err := os.MkdirAll(dir, os.ModePerm); err != nil {
			return err
		}
	}
	return nil
}

// 创建文件
// 可能存在跨越目录创建文件的风险
func CreateFile(path string) error {
	if FileExist(path) {
		return nil
	}

	if err := Mkdir(path); err != nil {
		return err
	}

	fi, err := os.Create(path)
	if err != nil {
		return err
	}
	defer fi.Close()

	return nil
}

// 类似php的array_column($a, null, 'key')
func ListToMap(list interface{}, key string) map[string]interface{} {
	v := reflect.ValueOf(list)
	if v.Kind() != reflect.Slice {
		return nil
	}

	res := make(map[string]interface{}, v.Len())
	for i := 0; i < v.Len(); i++ {
		item := v.Index(i).Interface()
		itemValue := reflect.ValueOf(item)
		keyValue := itemValue.FieldByName(key)
		if keyValue.IsValid() && keyValue.Kind() == reflect.String {
			res[keyValue.String()] = item
		}
	}

	return res
}

// 判断值是否在切片中
func InSlice[T comparable](slice []T, value T) int {
	for i, item := range slice {
		if item == value {
			return i
		}
	}
	return -1
}

// 判断一个字符串是否包含多个子字符串中的任意一个
func ContainsAny(str string, substrs []string) bool {
	for _, substr := range substrs {
		if strings.Contains(str, substr) {
			return true
		}
	}
	return false
}

// 取两个切片的交集
func Union[T string | Number](slice1, slice2 []T) []T {
	// 创建一个空的哈希集合用于存储第一个切片的元素
	set1 := make(map[T]struct{})
	for _, elem := range slice1 {
		set1[elem] = struct{}{}
	}

	// 创建一个空的哈希集合用于存储交集
	intersectionSet := make(map[T]struct{})
	for _, elem := range slice2 {
		if _, exists := set1[elem]; exists {
			intersectionSet[elem] = struct{}{}
		}
	}

	// 将交集哈希集合中的所有元素转换为一个切片
	result := make([]T, 0, len(intersectionSet))
	for elem := range intersectionSet {
		result = append(result, elem)
	}

	return result
}

// 生成文件路径和文件名
func ReadyFile(staticDir string, loc *time.Location, fileExt ...string) (string, string) {
	ext := ""
	if len(fileExt) > 0 {
		ext = fileExt[0]
	}

	n, err := crand.Int(crand.Reader, big.NewInt(100))
	if err != nil {
		return "", ""
	}

	return filepath.Join(staticDir, time.Now().In(loc).Format("2006/01/")) + "/", Hash(strconv.FormatInt(time.Now().UnixNano()+n.Int64(), 10))[:10] + ext
}

// GetTTLWithJitter 为缓存TTL增加随机抖动，防止缓存雪崩
func GetTTLWithJitter(baseTTLInSeconds int64) time.Duration {
	if baseTTLInSeconds <= 0 {
		return 0
	}
	// 添加一个最多为基础TTL 10% 的随机抖动
	jitter := rand.Int63n(baseTTLInSeconds / 10)
	return time.Duration(baseTTLInSeconds+jitter) * time.Second
}

// ParseDateFromLogFileName 从日志文件名中解析日期
// 文件名格式如: gin.log.2025-10-28, run.log.2025-10-28
func ParseDateFromLogFileName(filename string, loc *time.Location) (time.Time, bool) {
	parts := strings.Split(filename, ".")
	if len(parts) < 2 {
		return time.Time{}, false
	}

	// 日期部分应在最后
	dateStr := parts[len(parts)-1]
	// 使用 "2006-01-02" 格式解析
	t, err := time.ParseInLocation("2006-01-02", dateStr, loc)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
