package healer

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/aviddiviner/go-murmur"
	"github.com/golang/glog"
)

type Producer struct {
	config                    *ProducerConfig
	topic                     string
	leaderIDToSimpleProducers map[int32]*SimpleProducer
	currentProducer           *SimpleProducer
	currentPartitionID        int32
	brokers                   *Brokers
	topicMeta                 TopicMetadata
}

func NewProducer(topic string, config *ProducerConfig) *Producer {
	var err error
	err = config.checkValid()
	if err != nil {
		glog.Errorf("producer config error: %s", err)
		return nil
	}

	p := &Producer{
		config:                    config,
		topic:                     topic,
		leaderIDToSimpleProducers: make(map[int32]*SimpleProducer),
	}

	brokerConfig := getBrokerConfigFromProducerConfig(config)
	p.brokers, err = NewBrokersWithConfig(config.BootstrapServers, brokerConfig)
	if err != nil {
		glog.Errorf("init brokers error: %s", err)
		return nil
	}

	err = p.refreshTopicMeta()
	if err != nil {
		glog.Errorf("get metadata of topic %s error: %v", p.topic, err)
		return nil
	}

	go func() {
		for range time.NewTicker(time.Duration(config.MetadataMaxAgeMS) * time.Millisecond).C {
			err := p.refreshTopicMeta()
			if err != nil {
				glog.Errorf("refresh metadata error in producer %s ticker: %v", p.topic, err)
			}
		}
	}()

	return p
}

// refresh metadata and currentProducer
func (p *Producer) refreshTopicMeta() error {
	for i := 0; i < p.config.FetchTopicMetaDataRetrys; i++ {
		metadataResponse, err := p.brokers.RequestMetaData(p.config.ClientID, []string{p.topic})
		if err != nil {
			glog.Errorf("get topic metadata error: %s", err)
			continue
		}
		if len(metadataResponse.TopicMetadatas) == 0 {
			glog.Errorf("get topic metadata error: %s", errNoTopicsInMetadata)
			continue
		}
		p.topicMeta = metadataResponse.TopicMetadatas[0]

		rand.Seed(time.Now().UnixNano())
		validPartitionID := make([]int32, 0)
		for _, partition := range p.topicMeta.PartitionMetadatas {
			if partition.PartitionErrorCode == 0 {
				validPartitionID = append(validPartitionID, partition.PartitionID)
			}
		}
		partitionID := validPartitionID[rand.Int31n(int32(len(validPartitionID)))]
		sp := NewSimpleProducer(p.topic, partitionID, p.config)
		if sp == nil {
			glog.Errorf("could not update current simple producer from the %s-%d. use the previous one", p.topic, partitionID)
			return nil
		}
		if p.currentProducer == nil {
			glog.Infof("update current simple producer to %s", sp.leader.GetAddress())
		} else {
			glog.Infof("update current simple producer from the %s to %s", p.currentProducer.leader.GetAddress(), sp.leader.GetAddress())
			p.currentProducer.Close()
		}
		p.currentProducer = sp

		return nil
	}
	return fmt.Errorf("failed to get topic meta of %s after %d tries", p.topic, p.config.FetchTopicMetaDataRetrys)
}

// getLeader return the leader broker id of the partition from metadata cache
func (p *Producer) getLeader(pid int32) (int32, error) {
	for _, partition := range p.topicMeta.PartitionMetadatas {
		if partition.PartitionID == pid {
			if partition.PartitionErrorCode == 0 {
				return partition.Leader, nil
			}
			return -1, getErrorFromErrorCode(partition.PartitionErrorCode)
		}
	}
	return -1, fmt.Errorf("partition %s-%d not found in metadata", p.topic, pid)
}

func (p *Producer) getSimpleProducer(key []byte) (*SimpleProducer, error) {
	if key == nil {
		return p.currentProducer, nil
	}

	partitionID := int32(murmur.MurmurHash2(key, 0) % uint32(len(p.topicMeta.PartitionMetadatas)))

	leaderID, err := p.getLeader(partitionID)
	if err != nil {
		return nil, err
	}

	if sp, ok := p.leaderIDToSimpleProducers[leaderID]; ok {
		return sp, nil
	}

	sp := NewSimpleProducer(p.topic, partitionID, p.config)
	p.leaderIDToSimpleProducers[partitionID] = sp
	return sp, nil
}

// AddMessage add message to the producer, if key is nil, use current simple producer, else use the simple producer of the partition of the key
// if the simple producer of the partition of the key not exist, create a new one
// if the simple producer closed, retry 3 times
func (p *Producer) AddMessage(key []byte, value []byte) error {
	for i := 0; i < 3; i++ {
		simpleProducer, err := p.getSimpleProducer(key)
		if err != nil {
			return err
		}
		err = simpleProducer.AddMessage(key, value)
		if err == ErrProducerClosed { // maybe current simple-producer closed in ticker, retry
			continue
		}
		return err
	}
	return nil
}

// Close close all simple producers in the console producer
func (p *Producer) Close() {
	for _, sp := range p.leaderIDToSimpleProducers {
		sp.Close()
	}
}
