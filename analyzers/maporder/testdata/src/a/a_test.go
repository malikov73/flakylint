package a

import (
	"reflect"
	"slices"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func normalize(s []string) []string { return s }

// --- flagged ---------------------------------------------------------------

func TestClassicKeys(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2}
	var got []string
	for k := range m {
		got = append(got, k)
	}
	assert.Equal(t, []string{"a", "b"}, got) // want `assertion depends on map iteration order`
}

func TestValues(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2}
	var got []int
	for _, v := range m {
		got = append(got, v)
	}
	require.Equal(t, []int{1, 2}, got) // want `assertion depends on map iteration order`
}

func TestAccFirstArg(t *testing.T) {
	m := map[string]int{"a": 1}
	var got []string
	for k := range m {
		got = append(got, k)
	}
	assert.Equal(t, got, []string{"a"}) // want `assertion depends on map iteration order`
}

func TestStringAccum(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2}
	s := ""
	for k := range m {
		s += k
	}
	assert.Equal(t, "ab", s) // want `assertion depends on map iteration order`
}

func TestReflectDeepEqual(t *testing.T) {
	m := map[string]int{"a": 1}
	var got []string
	for k := range m {
		got = append(got, k)
	}
	if !reflect.DeepEqual(got, []string{"a"}) { // want `assertion depends on map iteration order`
		t.Errorf("mismatch")
	}
}

func TestSlicesEqual(t *testing.T) {
	m := map[string]int{"a": 1}
	var got []string
	for k := range m {
		got = append(got, k)
	}
	if !slices.Equal([]string{"a"}, got) { // want `assertion depends on map iteration order`
		t.Error("mismatch")
	}
}

func TestSubtest(t *testing.T) {
	t.Run("sub", func(t *testing.T) {
		m := map[string]int{"a": 1}
		var got []string
		for k := range m {
			got = append(got, k)
		}
		assert.Equal(t, []string{"a"}, got) // want `assertion depends on map iteration order`
	})
}

// An accumulator declared OUTSIDE the map range but appended via an inner
// slice-range loop still interleaves across map iterations, so it must flag.
func TestOuterAccInnerSliceRange(t *testing.T) {
	byGroup := map[string][]string{"g1": {"a", "b"}, "g2": {"c"}}
	var got []string
	for _, uids := range byGroup {
		for _, uid := range uids {
			got = append(got, uid)
		}
	}
	assert.Equal(t, []string{"a", "b", "c"}, got) // want `assertion depends on map iteration order`
}

// --- silent ----------------------------------------------------------------

func TestSortStrings(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2}
	var got []string
	for k := range m {
		got = append(got, k)
	}
	sort.Strings(got)
	assert.Equal(t, []string{"a", "b"}, got)
}

func TestSlicesSort(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2}
	var got []string
	for k := range m {
		got = append(got, k)
	}
	slices.Sort(got)
	assert.Equal(t, []string{"a", "b"}, got)
}

// A sort silences only assertions positioned after it. Here the assert runs on
// the still-unsorted accumulator, so it flakes; the later sort is irrelevant.
func TestSortAfterAssert(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2}
	var got []string
	for k := range m {
		got = append(got, k)
	}
	assert.Equal(t, []string{"a", "b"}, got) // want `assertion depends on map iteration order`
	sort.Strings(got)
}

// The assert reads the accumulator before the map-range loop fills it, so its
// value cannot depend on iteration order — silent (review counterexample).
func TestAssertBeforeLoop(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2}
	var got []string
	assert.Equal(t, []string{"a", "b"}, got)
	for k := range m {
		got = append(got, k)
	}
}

// The accumulator appears only as a testify message argument (index > 2), which
// is message-only, not an order-sensitive comparison — silent.
func TestAccInMsgArgsOnly(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2}
	var got []string
	for k := range m {
		got = append(got, k)
	}
	n := len(got)
	assert.Equal(t, 2, n, "accumulated %v", got)
}

func TestElementsMatch(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2}
	var got []string
	for k := range m {
		got = append(got, k)
	}
	assert.ElementsMatch(t, []string{"a", "b"}, got)
}

func TestLen(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2}
	var got []string
	for k := range m {
		got = append(got, k)
	}
	assert.Len(t, got, 2)
}

func TestEscapeHelper(t *testing.T) {
	m := map[string]int{"a": 1}
	var got []string
	for k := range m {
		got = append(got, k)
	}
	_ = normalize(got)
	assert.Equal(t, []string{"a"}, got)
}

func TestEscapeReassign(t *testing.T) {
	m := map[string]int{"a": 1}
	var got []string
	for k := range m {
		got = append(got, k)
	}
	other := got
	_ = other
	assert.Equal(t, []string{"a"}, got)
}

func TestEscapeAddr(t *testing.T) {
	m := map[string]int{"a": 1}
	var got []string
	for k := range m {
		got = append(got, k)
	}
	_ = &got
	assert.Equal(t, []string{"a"}, got)
}

func TestRangeSlice(t *testing.T) {
	src := []string{"a", "b"}
	var got []string
	for _, v := range src {
		got = append(got, v)
	}
	assert.Equal(t, []string{"a", "b"}, got)
}

func TestNoAssert(t *testing.T) {
	m := map[string]int{"a": 1}
	var got []string
	for k := range m {
		got = append(got, k)
	}
	_ = got
}

func TestMixedProvenance(t *testing.T) {
	m := map[string]int{"a": 1}
	var got []string
	got = append(got, "x")
	for k := range m {
		got = append(got, k)
	}
	assert.Equal(t, []string{"x", "a"}, got)
}

// Per-group accumulators declared inside the map-range body are fresh on every
// iteration, so their internal order cannot depend on map iteration order. The
// map range only picks which group is asserted; the order comes from ranging
// the slice value. (Regression: grafana schedule_unit_test.go:650.)
func TestPerIterationAccumulator(t *testing.T) {
	byGroup := map[string][]string{"g1": {"a", "b"}, "g2": {"c"}}
	for _, uids := range byGroup {
		var got []string
		for _, uid := range uids {
			got = append(got, uid)
		}
		assert.Equal(t, uids, got)
	}
}

func TestSumNotStringAccum(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2}
	sum := 0
	for _, v := range m {
		sum += v
	}
	assert.Equal(t, 3, sum)
}
