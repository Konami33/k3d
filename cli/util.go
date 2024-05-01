package run

import (
	"fmt"
	"math/rand"
	"net"
	"strconv"
	"strings"
	"time"
)

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

const clusterNameMaxSize int = 35

// These constants are used for calculating the random index within the letterBytes string.
// Calculation:
// letterIdxBits = 6:
// This means that each letter index can be represented using 6 bits.
// letterIdxMask = 1<<6 - 1:
// Using bitwise left shift (<<), we shift 1 six positions to the left, which results in 1000000.
// Then, we subtract 1 from this value, which sets all the bits to 1 except the leftmost bit, resulting in 111111.
// So, letterIdxMask is 63 (111111 in binary), which is the maximum value that can be represented with 6 bits.
// letterIdxMax = 63 / 6:
// We divide 63 (the maximum value representable with 6 bits) by 6, which gives 10.
// This means that there are 10 unique indices that can be represented with 6 bits.
const (
	letterIdxBits = 6                    // 6 bits to represent a letter index
	letterIdxMask = 1<<letterIdxBits - 1 // 111111
	letterIdxMax  = 63 / letterIdxBits   // letterIdxMax := 10
)

// time.Now().UnixNano() returns the current time in nanoseconds since January 1, 1970 .
// rand.NewSource creates a new random number generator source using an int64 seed value.
var src = rand.NewSource(time.Now().UnixNano())

// GenerateRandomString thanks to https://stackoverflow.com/a/31832326/6450189
func GenerateRandomString(n int) string {

	//A Builder is used to efficiently build a string using [Builder.Write]
	sb := strings.Builder{}
	// Grow grows sb's capacity, if necessary, to guarantee space for another n bytes. After Grow(n), at least n bytes can be written to b without another allocation. If n is negative, Grow panics.
	sb.Grow(n)
	// A src.Int63() generates 63 random bits, enough for letterIdxMax characters!
	for i, cache, remain := n-1, src.Int63(), letterIdxMax; i >= 0; {
		// i := n-1, n is the length of the desired random string.
		// cache := random 63-bit integer generated by src.Int63().
		// remain := letterIdxMax. (10)
		// condition i>=0

		//If remain is 0, it means we have exhausted all the bits in the current cache, so we need to generate a new random 63-bit integer (cache) and reset remain to letterIdxMax.
		if remain == 0 {
			cache, remain = src.Int63(), letterIdxMax
		}
		// This calculates the index (idx) by bitwise AND operation between cache and letterIdxMask, which ensures that idx falls within the range of valid indices for the letterBytes slice.
		if idx := int(cache & letterIdxMask); idx < len(letterBytes) {
			sb.WriteByte(letterBytes[idx])
			i--
		}
		// The purpose of this operation is to prepare cache for the next iteration of the loop. By shifting the bits to the right, some of the bits used in the previous iteration are discarded, making room for new random bits to be generated in subsequent iterations.
		cache >>= letterIdxBits
		remain--
	}
	return sb.String()
}

// Make sure a cluster name is also a valid host name according to RFC 1123.
// We further restrict the length of the cluster name to shorter than 'clusterNameMaxSize'
// so that we can construct the host names based on the cluster name, and still stay
// within the 64 characters limit.
func CheckClusterName(name string) error {
	if err := ValidateHostname(name); err != nil {
		return fmt.Errorf("[ERROR] Invalid cluster name\n%+v", ValidateHostname(name))
	}
	if len(name) > clusterNameMaxSize {
		return fmt.Errorf("[ERROR] Cluster name is too long (%d > %d)", len(name), clusterNameMaxSize)
	}
	return nil
}

// ValidateHostname ensures that a cluster name is also a valid host name according to RFC 1123.
func ValidateHostname(name string) error {
	// Hostname mustbe defined
	if len(name) == 0 {
		return fmt.Errorf("[ERROR] Hostname [%s] must not be empty", name)
	}
	if name[0] == '-' || name[len(name)-1] == '-' {
		return fmt.Errorf("[ERROR] Hostname [%s] must not start or end with - (dash)", name)
	}

	for _, c := range name {
		switch {
		case '0' <= c && c <= '9':
		case 'a' <= c && c <= 'z':
		case 'A' <= c && c <= 'Z':
		case c == '-':
			continue
		default:
			return fmt.Errorf("[ERROR] Hostname [%s] contains characters other than 'Aa-Zz', '0-9' or '-'", name)

		}
	}
	return nil
}

type apiPort struct {
	Host string
	Port string
}

func parseApiPort(portSpec string) (*apiPort, error) {

	var port *apiPort
	// 80:8080 --> {"80", "8080
	// 80 --> {"80", ""}
	split := strings.Split(portSpec, ":")
	if len(split) > 2 {
		return nil, fmt.Errorf("api-port format error")
	}
	// If no port is specified
	if len(split) == 1 {
		port = &apiPort{Port: split[0]}
	} else {
		// Make sure 'host' can be resolved to an IP address
		// LookupHost looks up the given host using the local resolver. It returns a slice of that host's addresses.
		_, err := net.LookupHost(split[0])
		if err != nil {
			return nil, err
		}
		port = &apiPort{Host: split[0], Port: split[1]}
	}

	// Verify 'port' is an integer and within port ranges
	p, err := strconv.Atoi(port.Port)
	if err != nil {
		return nil, err
	}

	if p < 0 || p > 65535 {
		return nil, fmt.Errorf("ERROR: --api-port port value out of range")
	}

	return port, nil
}
