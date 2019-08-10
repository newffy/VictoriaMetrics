package opentsdbhttp

import (
	"fmt"
	"github.com/valyala/fastjson"
	"unsafe"
)

const SECOND_MASK int64 = 0x7FFFFFFF00000000


// Rows contains parsed OpenTSDB rows.
type Rows struct {
	Rows []Row

	tagsPool []Tag
}

// Reset resets rs.
func (rs *Rows) Reset() {
	// Release references to objects, so they can be GC'ed.

	for i := range rs.Rows {
		rs.Rows[i].reset()
	}
	rs.Rows = rs.Rows[:0]

	for i := range rs.tagsPool {
		rs.tagsPool[i].reset()
	}
	rs.tagsPool = rs.tagsPool[:0]
}

// Unmarshal unmarshals OpenTSDB rows from http POST body.
//
// See http://opentsdb.net/docs/build/html/api_http/put.html
//
// s must be unchanged until rs is in use.
func (rs *Rows) Unmarshal(av *fastjson.Value) error {
	var err error
	rs.Rows, rs.tagsPool, err = unmarshalRows(rs.Rows[:0], av, rs.tagsPool[:0])
	if err != nil {
		return err
	}
	return err
}

// Row is a single OpenTSDB row.
type Row struct {
	Metric    string
	Tags      []Tag
	Value     float64
	Timestamp int64
}

func (r *Row) reset() {
	r.Metric = ""
	r.Tags = nil
	r.Value = 0
	r.Timestamp = 0
}

func ob2s(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

func (r *Row) unmarshal(o *fastjson.Value, tagsPool []Tag) ([]Tag, error) {
	r.reset()
	m := o.GetStringBytes("metric")
	if m == nil {
		return tagsPool, fmt.Errorf("missing `metric` field in %s", o)
	}
	r.Metric = ob2s(m)

	rawTs := o.Get("timestamp")
	if rawTs != nil {
		ts, err := rawTs.Int64()
		if err != nil {
			// if timestamp has fractional part
			tsF, err := rawTs.Float64()
			if err != nil {
				return tagsPool, fmt.Errorf("invalid `timestamp` field in %s", o)
			}
			//probably this is millisecs, though logic should be improved (microseconds?)
			ts = int64(tsF * 1000)
		}
		// according to opentsdb/src/core/IncomingDataPoints.java, addPointInternal
		if ts & SECOND_MASK == 0 {
			ts *=  1000
		}
		r.Timestamp = ts
	} else {
		return tagsPool, fmt.Errorf("missing `timestamp` field in %s", o)
	}

	rawV := o.Get("value")
	if rawV != nil {
		v, err := rawV.Float64()
		if err != nil {
			return tagsPool, fmt.Errorf("invalid `value` field in %s", o)
		}
		r.Value = v
	} else {
		return tagsPool, fmt.Errorf("missing `value` field in %s", o)
	}

	rawTags := o.GetObject("tags")

	if rawTags == nil {
		return tagsPool, fmt.Errorf("missing `tags` field in %s", o)
	}

	tagsStart := len(tagsPool)
	tagsPool = unmarshalTags(tagsPool, rawTags)

	tags := tagsPool[tagsStart:]
	r.Tags = tags[:len(tags):len(tags)]
	return tagsPool, nil
}

func unmarshalRows(dst []Row, av *fastjson.Value, tagsPool []Tag) ([]Row, []Tag, error) {
	var err error
	if av == nil {
		err = fmt.Errorf("cannot unmarshal OpenTSDB body, it is empty")
		return dst, tagsPool, err
	}
	if av.Type() == fastjson.TypeObject {
		if cap(dst) > len(dst) {
			dst = dst[:len(dst)+1]
		} else {
			dst = append(dst, Row{})
		}
		r := &dst[len(dst)-1]
		tagsPool, err = r.unmarshal(av, tagsPool)
		if err != nil {
			err = fmt.Errorf("cannot unmarshal OpenTSDB body %s: %s", av, err)
			return dst, tagsPool, err
		}
		return dst, tagsPool, nil
	} else if av.Type() == fastjson.TypeArray {
		a, _ := av.Array()
		for _, e := range a {
			if cap(dst) > len(dst) {
				dst = dst[:len(dst)+1]
			} else {
				dst = append(dst, Row{})
			}
			r := &dst[len(dst)-1]
			tagsPool, err = r.unmarshal(e, tagsPool)
			if err != nil {
				err = fmt.Errorf("cannot unmarshal OpenTSDB body %s: %s", e, err)
				return dst, tagsPool, err
			}
		}
		return dst, tagsPool, nil
	} else {
		err = fmt.Errorf("cannot unmarshal OpenTSDB body, type is not object or array: %s", av)
		return dst, tagsPool, err
	}
}

func unmarshalTags(dst []Tag, tags *fastjson.Object) []Tag {
	tags.Visit(func(k []byte, v *fastjson.Value) {
		if cap(dst) > len(dst) {
			dst = dst[:len(dst)+1]
		} else {
			dst = append(dst, Tag{})
		}
		tag := &dst[len(dst)-1]

		tv := v.GetStringBytes()
		if tv == nil {
			dst = dst[:len(dst)-1]
			return
		}
		tag.Key = ob2s(k)
		tag.Value = ob2s(tv)
	})
	return dst
}

// Tag is an OpenTSDB tag.
type Tag struct {
	Key   string
	Value string
}

func (t *Tag) reset() {
	t.Key = ""
	t.Value = ""
}
