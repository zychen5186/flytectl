package bubbletea

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/flyteorg/flytectl/pkg/filters"
	"github.com/flyteorg/flytectl/pkg/printer"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
)

type DataCallback func(filter filters.Filters) []proto.Message

type printTableProto struct{ proto.Message }

const (
	msgPerBatch       = 100 // Please set msgPerBatch as a multiple of msgPerPage
	msgPerPage        = 10
	pagePerBatch      = msgPerBatch / msgPerPage
	prefetchThreshold = pagePerBatch - 1
	localBatchLimit   = 10 // Please set localBatchLimit at least 2
)

var (
	// Record the index of the first and last batch that is in cache
	firstBatchIndex = 0
	lastBatchIndex  = 0
	batchLen        = make(map[int]int)
	// Callback function used to fetch data from the module that called bubbletea pagination.
	callback DataCallback
	// The header of the table
	listHeader []printer.Column
	// Avoid fetching back and forward at the same time
	mutex sync.Mutex
)

func (p printTableProto) MarshalJSON() ([]byte, error) {
	marshaller := jsonpb.Marshaler{Indent: "\t"}
	buf := new(bytes.Buffer)
	err := marshaller.Marshal(buf, p.Message)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func _min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func getSliceBounds(m *pageModel) (start int, end int) {
	start = (m.paginator.Page - firstBatchIndex*pagePerBatch) * msgPerPage
	end = _min(start+msgPerPage, len(*m.items))
	return start, end
}

func getTable(m *pageModel) (string, error) {
	start, end := getSliceBounds(m)
	curShowMessage := (*m.items)[start:end]
	printTableMessages := make([]*printTableProto, 0, len(curShowMessage))
	for _, m := range curShowMessage {
		printTableMessages = append(printTableMessages, &printTableProto{Message: m})
	}

	jsonRows, err := json.Marshal(printTableMessages)
	if err != nil {
		return "", fmt.Errorf("failed to marshal proto messages")
	}

	var buf strings.Builder
	p := printer.Printer{}
	if err := p.JSONToTable(&buf, jsonRows, listHeader); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func getMessageList(batchIndex int) []proto.Message {
	mutex.Lock()
	spin = true
	defer func() {
		spin = false
		mutex.Unlock()
	}()

	msg := callback(filters.Filters{
		Limit:  msgPerBatch,
		Page:   int32(batchIndex + 1),
		SortBy: "created_at",
		Asc:    false,
	})

	batchLen[batchIndex] = len(msg)

	return msg
}

const (
	forward int = iota
	backward
)

type newDataMsg struct {
	newItems       []proto.Message
	batchIndex     int
	fetchDirection int
}

func fetchDataCmd(batchIndex int, fetchDirection int) tea.Cmd {
	return func() tea.Msg {
		msg := newDataMsg{
			newItems:       getMessageList(batchIndex),
			batchIndex:     batchIndex,
			fetchDirection: fetchDirection,
		}
		return msg
	}
}

func countTotalPages() int {
	sum := 0
	for i := 0; i < lastBatchIndex+1; i++ {
		sum += batchLen[i]
	}
	return sum
}
