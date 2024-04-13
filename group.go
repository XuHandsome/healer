package healer

import "encoding/binary"

type PartitionAssignment struct {
	Topic      string
	Partitions []int32
}

// MemberAssignment will be encoded to []byte that used as memeber of GroupAssignment in Sync Request.
// and sync and group response returns []byte that can be decoded to MemberAssignment
type MemberAssignment struct {
	Version              int16
	PartitionAssignments []*PartitionAssignment
	UserData             []byte
}

func (memberAssignment *MemberAssignment) Length() int {
	length := 2 + 4
	for _, p := range memberAssignment.PartitionAssignments {
		length += 2 + len(p.Topic)
		length += 4 + len(p.Partitions)*4
	}
	length += 4 + len(memberAssignment.UserData)
	return length
}

func (memberAssignment *MemberAssignment) Encode() []byte {
	payload := make([]byte, memberAssignment.Length())
	offset := 0

	binary.BigEndian.PutUint16(payload[offset:], uint16(memberAssignment.Version))
	offset += 2

	binary.BigEndian.PutUint32(payload[offset:], uint32(len(memberAssignment.PartitionAssignments)))
	offset += 4
	for _, p := range memberAssignment.PartitionAssignments {
		binary.BigEndian.PutUint16(payload[offset:], uint16(len(p.Topic)))
		offset += 2

		copy(payload[offset:], p.Topic)
		offset += len(p.Topic)

		binary.BigEndian.PutUint32(payload[offset:], uint32(len(p.Partitions)))
		offset += 4

		for _, partitionID := range p.Partitions {
			binary.BigEndian.PutUint32(payload[offset:], uint32(partitionID))
			offset += 4
		}
	}
	copy(payload[offset:], memberAssignment.UserData)

	return payload
}

func NewMemberAssignment(payload []byte) (*MemberAssignment, error) {
	if len(payload) == 0 {
		return nil, &emptyPayload
	}
	r := &MemberAssignment{}
	var (
		offset int   = 0
		count  int32 = 0
	)

	r.Version = int16(binary.BigEndian.Uint16(payload[offset:]))
	offset += 2

	count = int32(binary.BigEndian.Uint32(payload[offset:]))
	offset += 4
	r.PartitionAssignments = make([]*PartitionAssignment, count)

	for i := 0; i < int(count); i++ {
		r.PartitionAssignments[i] = &PartitionAssignment{}
		topicLength := int(binary.BigEndian.Uint16(payload[offset:]))
		offset += 2
		r.PartitionAssignments[i].Topic = string(payload[offset : offset+topicLength])
		offset += topicLength

		count := int(binary.BigEndian.Uint32(payload[offset:]))
		offset += 4
		r.PartitionAssignments[i].Partitions = make([]int32, count)
		for j := 0; j < count; j++ {
			p := int32(binary.BigEndian.Uint32(payload[offset:]))
			offset += 4
			r.PartitionAssignments[i].Partitions[j] = p
		}
	}

	count = int32(binary.BigEndian.Uint32(payload[offset:]))
	if count == -1 {
		return r, nil
	}
	offset += 4
	r.UserData = make([]byte, count)
	copy(r.UserData, payload[offset:])

	return r, nil
}

// TODO map
type GroupAssignment []struct {
	MemberID         string
	MemberAssignment []byte
}

// ProtocolMetadata is used in join request/response
type ProtocolMetadata struct {
	Version      uint16
	Subscription []string
	UserData     []byte
}

func (m *ProtocolMetadata) Length() int {
	length := 2 + 4
	for _, subscription := range m.Subscription {
		length += 2
		length += len(subscription)
	}
	length += 4 + len(m.UserData)
	return length
}

func (m *ProtocolMetadata) Encode() []byte {

	payload := make([]byte, m.Length())
	offset := 0
	binary.BigEndian.PutUint16(payload[offset:], m.Version)
	offset += 2

	binary.BigEndian.PutUint32(payload[offset:], uint32(len(m.Subscription)))
	offset += 4

	for _, subscription := range m.Subscription {
		binary.BigEndian.PutUint16(payload[offset:], uint16(len(subscription)))
		offset += 2
		copy(payload[offset:], subscription)
		offset += len(subscription)
	}
	binary.BigEndian.PutUint32(payload[offset:], uint32(len(m.UserData)))
	offset += 4
	copy(payload[offset:], m.UserData)

	return payload
}

func NewProtocolMetadata(payload []byte) *ProtocolMetadata {
	var (
		p      = &ProtocolMetadata{}
		offset = 0
	)
	p.Version = binary.BigEndian.Uint16(payload[offset:])
	offset += 2
	SubscriptionCount := binary.BigEndian.Uint32(payload[offset:])
	offset += 4
	p.Subscription = make([]string, SubscriptionCount)
	for i := range p.Subscription {
		l := int(binary.BigEndian.Uint16(payload[offset:]))
		offset += 2
		p.Subscription[i] = string(payload[offset : offset+l])
		offset += l
	}
	l := int32(binary.BigEndian.Uint32(payload[offset:]))
	offset += 4

	if l != -1 {
		p.UserData = make([]byte, int(l))
		copy(p.UserData, payload[offset:offset+int(l)])
	}

	return p
}
