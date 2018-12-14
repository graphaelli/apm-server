package ilm

import "github.com/elastic/beats/libbeat/common"

var policies = common.MapStr{
	"default":   beatDefaultPolicy,
	"shortTerm": shortTermPolicy,
	"longTerm":  longTermPolicy,
}

var beatDefaultPolicy = common.MapStr{
	"policy": common.MapStr{
		"phases": common.MapStr{
			"hot": common.MapStr{
				"actions": common.MapStr{
					"rollover": common.MapStr{
						"max_size": "50gb",
						"max_age":  "30d",
					},
				},
			},
		},
	},
}

var shortTermPolicy = common.MapStr{
	"policy": common.MapStr{
		"phases": common.MapStr{
			"hot": common.MapStr{
				"actions": common.MapStr{
					"rollover": common.MapStr{
						"max_size": "50gb",
						"max_age":  "1d",
					},
				},
			},
			"delete": common.MapStr{
				"min_age": "10d",
				"actions": common.MapStr{
					"delete": common.MapStr{},
				},
			},
		},
	},
}

var longTermPolicy = common.MapStr{
	"policy": common.MapStr{
		"phases": common.MapStr{
			"hot": common.MapStr{
				"actions": common.MapStr{
					"rollover": common.MapStr{
						"max_size": "50gb",
						"max_age":  "1d",
					},
				},
			},
			"delete": common.MapStr{
				"min_age": "1y",
				"actions": common.MapStr{
					"delete": common.MapStr{},
				},
			},
		},
	},
}
