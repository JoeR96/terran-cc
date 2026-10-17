package osrs

import (
	"bufio"
	"io"
	"strconv"
	"strings"
	"time"
)

// readProperties parses a Java .properties file: key=value lines with
// backslash escapes (\: \= \\ \uXXXX) and backslash-newline continuations.
func readProperties(r io.Reader) map[string]string {
	out := map[string]string{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	var pending string
	for sc.Scan() {
		line := pending + strings.TrimLeft(sc.Text(), " \t")
		pending = ""
		if strings.HasSuffix(line, `\`) && !strings.HasSuffix(line, `\\`) {
			pending = line[:len(line)-1]
			continue
		}
		if line == "" || line[0] == '#' || line[0] == '!' {
			continue
		}
		k, v := splitProperty(line)
		out[unescape(k)] = unescape(v)
	}
	return out
}

// splitProperty finds the first unescaped = or : separating key and value.
func splitProperty(line string) (string, string) {
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '\\':
			i++
		case '=', ':':
			return line[:i], line[i+1:]
		}
	}
	return line, ""
}

func unescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 == len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case 'u':
			if i+4 < len(s) {
				if r, err := strconv.ParseUint(s[i+1:i+5], 16, 32); err == nil {
					b.WriteRune(rune(r))
					i += 4
					continue
				}
			}
			b.WriteByte('u')
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// profileKey finds the RuneLite rsprofile key for a display name. One name
// can have several keys (e.g. after a world-type change); the one with the
// newest time-tracking stamp is the account in use.
func profileKey(props map[string]string, name string) string {
	best, bestStamp := "", int64(-1)
	for k, v := range props {
		key, ok := strings.CutPrefix(k, "rsprofile.rsprofile.")
		if !ok || !strings.HasSuffix(key, ".displayName") || !strings.EqualFold(strings.TrimSpace(v), name) {
			continue
		}
		key = strings.TrimSuffix(key, ".displayName")
		if s := newestStamp(props, key); s > bestStamp {
			best, bestStamp = key, s
		}
	}
	return best
}

func newestStamp(props map[string]string, key string) int64 {
	prefix := "timetracking.rsprofile." + key + "."
	var newest int64
	for k, v := range props {
		if strings.HasPrefix(k, prefix) {
			if _, s, ok := valueStamp(v); ok && s > newest {
				newest = s
			}
		}
	}
	return newest
}

// valueStamp splits a "value:unix" time-tracking entry.
func valueStamp(v string) (int, int64, bool) {
	a, b, ok := strings.Cut(v, ":")
	if !ok {
		return 0, 0, false
	}
	n, err1 := strconv.Atoi(a)
	s, err2 := strconv.ParseInt(b, 10, 64)
	return n, s, err1 == nil && err2 == nil
}

// timers is one account's time-tracking state.
type timers struct {
	Patches    map[string]stored // "region.varbit"
	BirdHouses []stored          // the four spaces, in varp order
	Contract   int               // item id of the contracted crop, 0 if none
	Offset     offset
	Autoweed   bool
	Newest     time.Time
}

type stored struct {
	Value int
	Stamp int64
}

// BirdHouseVarps are RuneLite's BIRDHOUSE_TRANSMIT_A..D.
var birdHouseVarps = []int{1626, 1627, 1628, 1629}

func readTimers(props map[string]string, key string) timers {
	p := "timetracking.rsprofile." + key + "."
	t := timers{Patches: map[string]stored{}}
	for k, v := range props {
		rest, ok := strings.CutPrefix(k, p)
		if !ok {
			continue
		}
		n, s, isStamp := valueStamp(v)
		if isStamp && s > t.Newest.Unix() {
			t.Newest = time.Unix(s, 0)
		}
		switch {
		case isStamp && strings.Count(rest, ".") == 1 && rest[0] >= '0' && rest[0] <= '9':
			t.Patches[rest] = stored{n, s}
		case rest == "contract":
			t.Contract, _ = strconv.Atoi(v)
		case rest == "farmTickOffset":
			t.Offset.Mins, _ = strconv.Atoi(v)
			t.Offset.Known = true
		case rest == "farmTickOffsetPrecision":
			t.Offset.Precision, _ = strconv.Atoi(v)
		case rest == "autoweed":
			t.Autoweed = v == "2" // RuneLite's Autoweed.ON ordinal
		}
	}
	for _, varp := range birdHouseVarps {
		n, s, _ := valueStamp(props[p+"birdhouse."+strconv.Itoa(varp)])
		t.BirdHouses = append(t.BirdHouses, stored{n, s})
	}
	return t
}
