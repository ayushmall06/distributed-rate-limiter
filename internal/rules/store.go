package rules

type Store struct {
	rules map[string]Rule
}

func NewStore() *Store {
	return &Store{
		rules: make(map[string]Rule),
	}
}

func (s *Store) Add(rule Rule) {
	key := rule.TenantId + ":" + rule.Resource
	s.rules[key] = rule
}

func (s *Store) Get(tenant, resource string) (Rule, bool) {
	key := tenant + ":" + resource
	rule, exists := s.rules[key]
	return rule, exists
}

func (s *Store) List() []Rule {
	result := make([]Rule, 0)

	for _, rule := range s.rules {
		result = append(result, rule)
	}
	return result
}

func (s *Store) Delete(tenant, resource string) {
	key := tenant + ":" + resource
	delete(s.rules, key)
}
