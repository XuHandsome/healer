package healer

import (
	"errors"
	"math/rand"
	"time"

	"github.com/aviddiviner/go-murmur"
	"github.com/golang/glog"
)

type Producer struct {
	config             *ProducerConfig
	topic              string
	simpleProducers    map[int32]*SimpleProducer
	currentProducer    *SimpleProducer
	currentPartitionID int32
	brokers            *Brokers
	topicMeta          *TopicMetadata
}

func NewProducer(topic string, config *ProducerConfig) *Producer {
	var err error
	err = config.checkValid()
	if err != nil {
		glog.Errorf("producer config error: %s", err)
		return nil
	}

	p := &Producer{
		config:          config,
		topic:           topic,
		simpleProducers: make(map[int32]*SimpleProducer),
	}

	brokerConfig := getBrokerConfigFromProducerConfig(config)
	p.brokers, err = NewBrokersWithConfig(config.BootstrapServers, brokerConfig)
	if err != nil {
		glog.Errorf("init brokers error: %s", err)
		return nil
	}

	err = p.refreshTopicMeta()
	if err != nil {
		glog.Error(err)
		return nil
	}
	rand.Seed(time.Now().Unix())
	p.refreshCurrentProducer()
	if p.currentProducer == nil {
		glog.Errorf("could not get simple producer for the topic, maybe topic does not exist")
		return nil
	}

	go func() {
		for range time.NewTicker(time.Duration(config.MetadataMaxAgeMS) * time.Millisecond).C {
			err := p.refreshTopicMeta()
			if err != nil {
				glog.Error(err)
			}
			p.refreshCurrentProducer()
		}
	}()

	return p
}

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
		return nil
	}
	return errors.New("failed to get topic meta after all tries")
}

func (p *Producer) refreshCurrentProducer() {
	var validPartitionID []int32
	for _, partition := range p.topicMeta.PartitionMetadatas {
		if partition.PartitionErrorCode == 0 {
			validPartitionID = append(validPartitionID, partition.PartitionID)
		}
	}
	if len(validPartitionID) == 0 {
		return
	}

	partitionID := validPartitionID[rand.Int31n(int32(len(validPartitionID)))]
	glog.V(5).Infof("current partitionID is %d", partitionID)
	sp := NewSimpleProducer(p.topic, partitionID, p.config)
	if sp == nil {
		glog.Error("could not referesh current simple producer")
	}
	p.currentProducer = sp
	p.simpleProducers[partitionID] = p.currentProducer
}

func (p *Producer) AddMessage(key []byte, value []byte) error {
	if len(key) == 0 {
		return p.currentProducer.AddMessage(key, value)
	}
	partitionID := int32(murmur.MurmurHash2(key, 0) % uint32(len(p.topicMeta.PartitionMetadatas)))
	if s, ok := p.simpleProducers[partitionID]; ok {
		return s.AddMessage(key, value)
	} else {
		simpleProducer := NewSimpleProducer(p.topic, partitionID, p.config)
		p.simpleProducers[partitionID] = simpleProducer
		return simpleProducer.AddMessage(key, value)
	}
}

func (p *Producer) Close() {
	for _, sp := range p.simpleProducers {
		sp.Close()
	}
}
