package ddb

import (
	"fmt"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/jpillora/backoff"
)

// NewScanner creates a new scanner with ddb connection
func NewScanner(config Config) *Scanner {
	config.setDefaults()

	return &Scanner{
		waitGroup: &sync.WaitGroup{},
		Config:    config,
	}
}

// Scanner is
type Scanner struct {
	waitGroup *sync.WaitGroup
	Config
}

// Start uses the handler function to process items for each of the total shard
func (s *Scanner) Start(handler Handler) {
	for i := 0; i < s.SegmentCount; i++ {
		s.waitGroup.Add(1)
		segment := (s.SegmentCount * s.SegmentOffset) + i
		go s.handlerLoop(handler, segment)
	}
}

// Wait pauses program until waitgroup is fulfilled
func (s *Scanner) Wait() {
	s.waitGroup.Wait()
}

func (s *Scanner) handlerLoop(handler Handler, segment int) {
	defer s.waitGroup.Done()

	var lastEvaluatedKey map[string]*dynamodb.AttributeValue

	bk := &backoff.Backoff{
		Max:    5 * time.Minute,
		Jitter: true,
	}

	for {
		// scan params
		params := &dynamodb.ScanInput{
			TableName:     aws.String(s.TableName),
			Segment:       aws.Int64(int64(segment)),
			TotalSegments: aws.Int64(int64(s.TotalSegments)),
			Limit:         aws.Int64(s.Config.Limit),
		}

		// last evaluated key
		if lastEvaluatedKey != nil {
			params.ExclusiveStartKey = lastEvaluatedKey
		}

		// scan, sleep if rate limited
		resp, err := s.Svc.Scan(params)
		if err != nil {
			fmt.Println(err)
			time.Sleep(bk.Duration())
			continue
		}
		bk.Reset()

		// call the handler function with items
		handler.HandleItems(resp.Items)

		// exit if last evaluated key empty
		if resp.LastEvaluatedKey == nil {
			break
		}

		// set last evaluated key
		lastEvaluatedKey = resp.LastEvaluatedKey
	}
}
