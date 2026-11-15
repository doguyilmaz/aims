package meter

import (
	"encoding/binary"
	"errors"
	"math"
	"strings"
	"time"
)

var errGrokWire = errors.New("Grok web usage response not verified")

type grokField struct {
	number, wire uint64
	value        uint64
	data         []byte
}

// Decode only declared messages. Unknown length-delimited fields are opaque;
// their bytes cannot accidentally become a percentage or reset timestamp.
func grokFields(data []byte) ([]grokField, error) {
	var fields []grokField
	for len(data) > 0 {
		tag, n := binary.Uvarint(data)
		if n <= 0 || tag>>3 == 0 || tag>>3 > (1<<29)-1 {
			return nil, errGrokWire
		}
		data = data[n:]
		f := grokField{number: tag >> 3, wire: tag & 7}
		switch f.wire {
		case 0:
			f.value, n = binary.Uvarint(data)
			if n <= 0 {
				return nil, errGrokWire
			}
			data = data[n:]
		case 1, 5:
			size := 8
			if f.wire == 5 {
				size = 4
			}
			if len(data) < size {
				return nil, errGrokWire
			}
			if size == 4 {
				f.value = uint64(binary.LittleEndian.Uint32(data))
			}
			data = data[size:]
		case 2:
			length, n := binary.Uvarint(data)
			if n <= 0 || length > uint64(len(data)-n) {
				return nil, errGrokWire
			}
			f.data, data = data[n:n+int(length)], data[n+int(length):]
		default:
			return nil, errGrokWire
		}
		fields = append(fields, f)
	}
	return fields, nil
}

func grokOne(fields []grokField, number, wire uint64) (grokField, error) {
	var found grokField
	for _, field := range fields {
		if field.number == number {
			if found.number != 0 || field.wire != wire {
				return grokField{}, errGrokWire
			}
			found = field
		}
	}
	return found, nil
}

func grokWireTime(data []byte) (time.Time, error) {
	fields, err := grokFields(data)
	if err != nil {
		return time.Time{}, err
	}
	seconds, err := grokOne(fields, 1, 0)
	if err != nil || seconds.number == 0 || seconds.value > math.MaxInt64 {
		return time.Time{}, errGrokWire
	}
	nanos, err := grokOne(fields, 2, 0)
	if err != nil || nanos.value > 999999999 {
		return time.Time{}, errGrokWire
	}
	return time.Unix(int64(seconds.value), int64(nanos.value)).UTC(), nil
}

func grokWebMessage(raw []byte, status string) ([]byte, error) {
	var message []byte
	trailer := false
	if status != "" && status != "0" {
		return nil, errGrokWire
	}
	for len(raw) > 0 {
		if len(raw) < 5 || trailer {
			return nil, errGrokWire
		}
		flags, length := raw[0], uint64(binary.BigEndian.Uint32(raw[1:5]))
		if length > uint64(len(raw)-5) {
			return nil, errGrokWire
		}
		frame := raw[5 : 5+int(length)]
		raw = raw[5+int(length):]
		switch flags {
		case 0:
			if message != nil || len(frame) == 0 {
				return nil, errGrokWire
			}
			message = frame
		case 128:
			trailer = true
			seen := false
			for line := range strings.SplitSeq(string(frame), "\r\n") {
				key, value, _ := strings.Cut(line, ":")
				if strings.EqualFold(key, "grpc-status") {
					if seen || strings.TrimSpace(value) != "0" {
						return nil, errGrokWire
					}
					seen, status = true, "0"
				}
			}
		default:
			return nil, errGrokWire
		}
	}
	if message == nil || status != "0" {
		return nil, errGrokWire
	}
	return message, nil
}

func parseGrokWeb(raw []byte, status string, expected grokPeriod, now time.Time) (grokWebReading, error) {
	if len(raw) > 65536 {
		return grokWebReading{}, errGrokWire
	}
	message, err := grokWebMessage(raw, status)
	if err != nil {
		return grokWebReading{}, err
	}
	root, err := grokFields(message)
	if err != nil {
		return grokWebReading{}, err
	}
	config, err := grokOne(root, 1, 2)
	if err != nil || config.number == 0 {
		return grokWebReading{}, errGrokWire
	}
	fields, err := grokFields(config.data)
	if err != nil {
		return grokWebReading{}, err
	}
	period, err := grokOne(fields, 8, 2)
	if err != nil || period.number == 0 {
		return grokWebReading{}, errGrokWire
	}
	pf, err := grokFields(period.data)
	if err != nil {
		return grokWebReading{}, err
	}
	typ, e1 := grokOne(pf, 1, 0)
	start, e2 := grokOne(pf, 2, 2)
	end, e3 := grokOne(pf, 3, 2)
	if e1 != nil || e2 != nil || e3 != nil || typ.value != 2 || start.number == 0 || end.number == 0 ||
		expected.Type != "USAGE_PERIOD_TYPE_WEEKLY" {
		return grokWebReading{}, errGrokWire
	}
	from, e1 := grokWireTime(start.data)
	to, e2 := grokWireTime(end.data)
	if e1 != nil || e2 != nil || !from.Equal(expected.Start) || !to.Equal(expected.End) ||
		from.After(now) || !to.After(now) || !from.Before(to) {
		return grokWebReading{}, errGrokWire
	}
	for number, want := range map[uint64]time.Time{4: from, 5: to} {
		legacy, err := grokOne(fields, number, 2)
		if err != nil {
			return grokWebReading{}, err
		}
		if legacy.number != 0 {
			stamp, err := grokWireTime(legacy.data)
			if err != nil || !stamp.Equal(want) {
				return grokWebReading{}, errGrokWire
			}
		}
	}
	percent, err := grokOne(fields, 1, 5)
	if err != nil {
		return grokWebReading{}, err
	}
	if percent.number != 0 {
		value := float64(math.Float32frombits(uint32(percent.value)))
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 100 {
			return grokWebReading{}, errGrokWire
		}
		return grokWebReading{Percent: value}, nil
	}
	// The web client's proto3 float has implicit zero presence. Only a complete,
	// active, matching weekly period qualifies, never the partial ACP JSON alone.
	unified, err := grokOne(fields, 11, 0)
	if err != nil || unified.value != 1 {
		return grokWebReading{}, errGrokWire
	}
	for _, f := range append(root, fields...) {
		if f.wire == 5 || (f.number == 7 && f.wire == 2) {
			return grokWebReading{}, errGrokWire
		}
	}
	return grokWebReading{ImplicitZero: true}, nil
}
