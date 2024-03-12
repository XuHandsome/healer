package healer

import (
	"sort"
)

// AssignmentStrategy is the interface for different assignment strategies, it returns GroupAssignment
type AssignmentStrategy interface {
	// generally topicMetadatas is returned by metaDataRequest sent by GroupConsumer
	Assign([]Member, []TopicMetadata) GroupAssignment
}

type rangeAssignmentStrategy struct {
}

// partitions in one topic
// XXX (3,5)=>[(0,2),(2,2),(4,1)]  (5,10)=>[(0,2), (2,2), (4,2), (6,2), (8,2)]
func (r *rangeAssignmentStrategy) assignPartitions(members []string, partitions []int32) map[string][]int32 {
	var (
		rst       = make(map[string][]int32)
		watershed = len(partitions) % len(members)
		ceil      int
		floor     = len(partitions) / len(members)
		start     = 0
		length    int
	)
	if watershed > 0 {
		ceil = floor + 1
	} else {
		ceil = floor
	}
	for i, member := range members {
		if i < watershed {
			length = ceil
		} else {
			length = floor
		}
		rst[member] = partitions[start : start+length]
		start += length
	}
	return rst
}

type ByPartitionID []int32

func (a ByPartitionID) Len() int           { return len(a) }
func (a ByPartitionID) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByPartitionID) Less(i, j int) bool { return a[i] < a[j] }

type ByMemberID []string

func (a ByMemberID) Len() int           { return len(a) }
func (a ByMemberID) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByMemberID) Less(i, j int) bool { return a[i] < a[j] }

// Assign is the implementation of AssignmentStrategy interface
func (r *rangeAssignmentStrategy) Assign(members []Member, topicMetadatas []TopicMetadata) GroupAssignment {

	topicPartitionsAssignments := make(map[string]map[string][]int32)
	for _, topicMetadata := range topicMetadatas {
		partitions := make([]int32, len(topicMetadata.PartitionMetadatas))
		for i, p := range topicMetadata.PartitionMetadatas {
			partitions[i] = p.PartitionID
		}
		sort.Sort(ByPartitionID(partitions))

		membersWithTheTopic := []string{}
		for _, member := range members {
			subscription := NewProtocolMetadata(member.MemberMetadata).Subscription
			for _, topic := range subscription {
				if topicMetadata.TopicName == topic {
					membersWithTheTopic = append(membersWithTheTopic, member.MemberID)
					sort.Sort(ByMemberID(membersWithTheTopic))
					break
				}
			}
		}
		topicPartitionsAssignments[topicMetadata.TopicName] = r.assignPartitions(membersWithTheTopic, partitions)
	}

	logger.V(5).Info("create tp assignments by RangeAssignmentStrategy", "assignment", topicPartitionsAssignments)

	groupAssignment := make([]struct {
		MemberID         string
		MemberAssignment []byte
	}, len(members))

	// memberAssignments is temporary, will transform to groupAssignment
	memberAssignments := make(map[string]*MemberAssignment)
	for _, member := range members {
		memberAssignments[member.MemberID] = &MemberAssignment{
			Version:              0,
			PartitionAssignments: make([]*PartitionAssignment, 0),
			UserData:             nil,
		}
	}

	for topic, partitionsAssignments := range topicPartitionsAssignments {
		for member, partitions := range partitionsAssignments {
			memberAssignments[member].PartitionAssignments = append(memberAssignments[member].PartitionAssignments, &PartitionAssignment{
				Topic:      topic,
				Partitions: partitions,
			})
		}
	}

	logger.V(1).Info("create member assignments", "memberAssignments", memberAssignments)

	i := 0
	for member, memberAssignment := range memberAssignments {
		groupAssignment[i] = struct {
			MemberID         string
			MemberAssignment []byte
		}{
			MemberID:         member,
			MemberAssignment: memberAssignment.Encode(),
		}
		i++
	}
	return groupAssignment
}
