package healer

import "github.com/golang/glog"

type MetaInfo struct {
	brokers        []*BrokerInfo
	topicMetadatas []TopicMetadata
	//groups         []string
}

type Helper struct {
	clientID string
	brokers  *Brokers
	metaInfo *MetaInfo
}

func NewHelperFromBrokers(brokers *Brokers, clientID string) *Helper {
	h := &Helper{
		clientID: clientID,
		brokers:  brokers,
		metaInfo: &MetaInfo{},
	}

	return h
}
func NewHelper(brokerList, clientID string, config *BrokerConfig) (*Helper, error) {
	//func NewBrokers(brokerList string, clientID string, connecTimeout int, timeout int) (*Brokers, error) {
	var (
		err error
	)
	h := &Helper{
		clientID: clientID,
	}

	h.brokers, err = NewBrokersWithConfig(brokerList, config)
	h.metaInfo = &MetaInfo{}
	if err != nil {
		return nil, err
	}

	return h, nil
}

func (h *Helper) UpdateMeta() error {
	metadataResponse, err := h.brokers.RequestMetaData(h.clientID, []string{})
	if err != nil {
		return err
	}

	h.metaInfo = &MetaInfo{}
	h.metaInfo.brokers = metadataResponse.Brokers
	h.metaInfo.topicMetadatas = metadataResponse.TopicMetadatas

	return nil
}

func (h *Helper) GetGroups() []string {
	err := h.UpdateMeta()
	if err != nil {
		glog.Errorf("update meta error:%s", err)
		return nil
	}
	groups := []string{}
	for _, brokerinfo := range h.metaInfo.brokers {
		broker, err := h.brokers.GetBroker(brokerinfo.NodeID)
		if err != nil {
			glog.Errorf("get broker [%d] error:%s", brokerinfo.NodeID, err)
			return nil
		}

		response, err := broker.requestListGroups(h.clientID)
		if err != nil {
			glog.Errorf("get group list from broker[%s] error:%s", broker.GetAddress(), err)
			return nil
		}
		for _, g := range response.Groups {
			groups = append(groups, g.GroupID)
		}
	}

	return groups
}
