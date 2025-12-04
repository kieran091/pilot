package pilot

import (
	"encoding/base64"
	"fmt"
	"hash/crc32"
	"sort"
	"sync"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

type uint32Slice []uint32

func (s uint32Slice) Len() int           { return len(s) }
func (s uint32Slice) Less(i, j int) bool { return s[i] < s[j] }
func (s uint32Slice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

// ConsistentHash implements a consistent hashing mechanism.
type ConsistentHash struct {
	replicas int
	keys     uint32Slice
	hashMap  map[uint32]string
	mu       sync.RWMutex
}

func NewConsistentHash(replicas int) *ConsistentHash {
	return &ConsistentHash{
		replicas: replicas,
		hashMap:  make(map[uint32]string),
	}
}

func (c *ConsistentHash) hashKey(key string) uint32 {
	return crc32.ChecksumIEEE([]byte(key))
}

func (c *ConsistentHash) add(hashNode string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i := range c.replicas {
		vNode := fmt.Sprintf("%s#%d", hashNode, i)
		h := c.hashKey(vNode)
		c.keys = append(c.keys, h)
		c.hashMap[h] = hashNode
	}

	sort.Sort(c.keys)
}

func (c *ConsistentHash) remove(hashNode string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i := range c.replicas {
		vNode := fmt.Sprintf("%s#%d", hashNode, i)
		h := c.hashKey(vNode)
		delete(c.hashMap, h)
	}

	newKeys := make(uint32Slice, 0, len(c.keys))
	for _, k := range c.keys {
		if _, ok := c.hashMap[k]; ok {
			newKeys = append(newKeys, k)
		}
	}

	c.keys = newKeys
	sort.Sort(c.keys)
}

func (c *ConsistentHash) get(key string) (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.keys) == 0 {
		return "", errors.New("no nodes in hash ring")
	}

	h := c.hashKey(key)
	idx := sort.Search(len(c.keys), func(i int) bool {
		return c.keys[i] >= h
	})
	if idx == len(c.keys) {
		idx = 0
	}

	hashNode := c.hashMap[c.keys[idx]]

	return hashNode, nil
}

type ServiceRegistry map[string]*ServiceEndpoint

type ServiceEndpoint struct {
	invokers  map[string]*GRPCInvoker
	instances []*ServiceInstance

	replicas int
	hashRing *ConsistentHash

	fds *descriptorpb.FileDescriptorSet

	mu sync.RWMutex
}

func newServiceEndpoint(replicas int, pb string) (*ServiceEndpoint, error) {
	pbDecode, err := base64.StdEncoding.DecodeString(pb)
	if err != nil {
		defaultLogger.Error().Err(err).Msg("failed to decode base64 proto descriptors")
		return nil, err
	}

	var fds descriptorpb.FileDescriptorSet
	if err = proto.Unmarshal(pbDecode, &fds); err != nil {
		defaultLogger.Error().Err(err).Msg("failed to unmarshal proto descriptors")
		return nil, err
	}

	return &ServiceEndpoint{
		invokers:  make(map[string]*GRPCInvoker),
		instances: make([]*ServiceInstance, 0),
		replicas:  replicas,
		hashRing:  NewConsistentHash(replicas),
		fds:       &fds,
	}, nil
}

// UpdateInstance updates the service instances and rebuilds the consistent hash ring.
func (se *ServiceEndpoint) UpdateInstance(instance *ServiceInstance) {
	if instance == nil {
		return
	}

	se.mu.Lock()
	defer se.mu.Unlock()

	if se.instances == nil {
		se.instances = make([]*ServiceInstance, 0)
	}
	if se.hashRing == nil {
		se.hashRing = NewConsistentHash(se.replicas)
	}
	if se.invokers == nil {
		se.invokers = make(map[string]*GRPCInvoker)
	}

	instanceExists := false
	var existingInstance *ServiceInstance
	for _, inst := range se.instances {
		if inst.Id == instance.Id {
			instanceExists = true
			existingInstance = inst
			break
		}
	}

	addressChanged := instanceExists && existingInstance.Addr != instance.Addr

	if !instanceExists {
		se.instances = append(se.instances, instance)
		hashNode := fmt.Sprintf("%s:%s", instance.Id, instance.Addr)
		se.hashRing.add(hashNode)

		invoker, err := NewGRPCInvoker(instance.Addr, se.fds)
		if err != nil {
			return
		}
		se.invokers[hashNode] = invoker
	} else if addressChanged {
		for i, inst := range se.instances {
			if inst.Id == instance.Id {
				se.instances[i] = instance
				break
			}
		}

		ring := NewConsistentHash(se.replicas)
		for _, inst := range se.instances {
			hashNode := fmt.Sprintf("%s:%s", inst.Id, inst.Addr)
			ring.add(hashNode)
		}
		se.hashRing = ring

		oldHashNode := fmt.Sprintf("%s:%s", existingInstance.Id, existingInstance.Addr)
		if invoker, exists := se.invokers[oldHashNode]; exists {
			_ = invoker.Close()
			delete(se.invokers, oldHashNode)
		}

		newHashNode := fmt.Sprintf("%s:%s", instance.Id, instance.Addr)
		invoker, err := NewGRPCInvoker(instance.Addr, se.fds)
		if err != nil {
			return
		}
		se.invokers[newHashNode] = invoker
	}
}

// GetInvoker returns an InvokerFunc that uses consistent hashing to select the appropriate service instance.
func (se *ServiceEndpoint) GetInvoker(service, method string) InvokerFunc {
	return func(c *Context, key string, input []byte) ([]byte, error) {
		se.mu.RLock()
		defer se.mu.RUnlock()

		if se.hashRing == nil {
			return nil, errors.New("hash ring not initialized")
		}

		hashNode, err := se.hashRing.get(key)
		if err != nil {
			return nil, err
		}

		invoker, exists := se.invokers[hashNode]
		if !exists {
			return nil, errors.Errorf("invoker not found for instance %s", hashNode)
		}

		return invoker.Invoke(c.ctx, service, method, input)
	}
}

func (se *ServiceEndpoint) Close() {
	se.mu.Lock()
	defer se.mu.Unlock()

	for _, invoker := range se.invokers {
		_ = invoker.Close()
	}

	se.invokers = make(map[string]*GRPCInvoker)
	se.instances = nil
	se.hashRing = nil
}
