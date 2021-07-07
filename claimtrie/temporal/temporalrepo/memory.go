package temporalrepo

type Memory struct {
	cache map[int32]map[string]bool
}

func NewMemory() *Memory {
	return &Memory{
		cache: map[int32]map[string]bool{},
	}
}

func (repo *Memory) SetNodesAt(names [][]byte, heights []int32) error {

	for i, height := range heights {
		c, ok := repo.cache[height]
		if !ok {
			c = map[string]bool{}
			repo.cache[height] = c
		}
		name := string(names[i])
		c[name] = true
	}

	return nil
}

func (repo *Memory) NodesAt(height int32) ([][]byte, error) {

	var names [][]byte

	for name := range repo.cache[height] {
		names = append(names, []byte(name))
	}

	return names, nil
}

func (repo *Memory) Close() error {
	return nil
}

func (repo *Memory) Flush() error {
	return nil
}
