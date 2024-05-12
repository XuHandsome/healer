package client

import (
	"github.com/childe/healer"
	"github.com/go-logr/logr"
)

type Client struct {
	clientID string

	logger logr.Logger

	brokers *healer.Brokers
}

// New creates a new Client
func New(bs, clientID string) (*Client, error) {
	var err error
	client := &Client{
		clientID: clientID,
		logger:   healer.GetLogger().WithName(clientID),
	}
	client.brokers, err = healer.NewBrokers(bs)
	return client, err
}

func (client *Client) WithLogger(logger logr.Logger) *Client {
	client.logger = logger
	return client
}

// Close closes the connections to kafka brokers
func (c *Client) Close() {
	c.brokers.Close()
}

// RefreshMetadata refreshes metadata for c.brokers
func (c *Client) RefreshMetadata() {
}

// ListGroups lists all consumer groups from all brokers
func (c *Client) ListGroups() (groups []string, err error) {
	for _, brokerinfo := range c.brokers.BrokersInfo() {
		broker, err := c.brokers.GetBroker(brokerinfo.NodeID)
		if err != nil {
			c.logger.Error(err, "get broker failed", "NodeID", brokerinfo.NodeID)
			return groups, err
		}

		response, err := broker.RequestListGroups(c.clientID)
		if err != nil {
			c.logger.Error(err, "get group list failed", "broker", broker.GetAddress())
			return groups, err
		}
		for _, g := range response.Groups {
			groups = append(groups, g.GroupID)
		}
	}
	return groups, nil
}

func (c *Client) DescribeLogDirs(topics []string) (map[int32]healer.DescribeLogDirsResponse, error) {
	c.logger.Info("describe logdirs", "topics", topics)

	meta, err := c.brokers.RequestMetaData(c.clientID, topics)
	if err != nil {
		return nil, err
	}

	type tp struct {
		Topic       string
		PartitionID int32
	}
	brokerPartitions := make(map[int32][]tp)
	for _, topic := range meta.TopicMetadatas {
		topicName := topic.TopicName
		for _, partition := range topic.PartitionMetadatas {
			pid := partition.PartitionID
			for _, b := range partition.Replicas {
				if _, ok := brokerPartitions[b]; !ok {
					brokerPartitions[b] = []tp{
						{
							Topic:       topicName,
							PartitionID: pid,
						},
					}
				} else {
					brokerPartitions[b] = append(brokerPartitions[b], tp{
						Topic:       topicName,
						PartitionID: pid,
					})
				}
			}
		}
	}

	c.logger.Info("broker partitions", "brokerPartitions", brokerPartitions)

	rst := make(map[int32]healer.DescribeLogDirsResponse)
	for b, topicPartitions := range brokerPartitions {
		req := healer.NewDescribeLogDirsRequest(c.clientID, nil)
		for _, tp := range topicPartitions {
			req.AddTopicPartition(tp.Topic, tp.PartitionID)
		}

		broker, err := c.brokers.GetBroker(b)
		if err != nil {
			return nil, err
		}
		resp, err := broker.RequestAndGet(req)
		if err != nil {
			c.logger.Error(err, "describe logdirs failed", "broker", broker.String())
			continue
		}

		topicSet := make(map[string]struct{})
		for _, t := range topics {
			topicSet[t] = struct{}{}
		}
		r := resp.(healer.DescribeLogDirsResponse)
		rs := r.Results
		for i := range rs {
			theTopics := rs[i].Topics
			filterdTopics := make([]healer.DescribeLogDirsResponseTopic, 0)
			for i := range theTopics {
				if _, ok := topicSet[theTopics[i].TopicName]; ok {
					filterdTopics = append(filterdTopics, theTopics[i])
				}
			}
			rs[i].Topics = filterdTopics
		}

		filteredTopicResults := make([]healer.DescribeLogDirsResponseResult, 0)
		for i := range rs {
			if len(rs[i].Topics) > 0 {
				filteredTopicResults = append(filteredTopicResults, rs[i])
			}
		}
		r.Results = filteredTopicResults
		rst[b] = resp.(healer.DescribeLogDirsResponse)
	}

	return rst, nil
}
