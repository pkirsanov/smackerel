package recipe

// Ingredient represents a single recipe ingredient from domain_data.
type Ingredient struct {
	Name        string `json:"name"`
	Quantity    string `json:"quantity"`
	Unit        string `json:"unit"`
	Preparation string `json:"preparation,omitempty"`
	Group       string `json:"group,omitempty"`
}

// Step represents a single recipe step from domain_data.
type Step struct {
	Number          int    `json:"number"`
	Instruction     string `json:"instruction"`
	DurationMinutes *int   `json:"duration_minutes,omitempty"`
	Technique       string `json:"technique,omitempty"`
}

// ScaledIngredient holds an ingredient with its scaled display quantity.
type ScaledIngredient struct {
	Name            string  `json:"name"`
	Quantity        string  `json:"quantity"`
	Unit            string  `json:"unit"`
	DisplayQuantity string  `json:"display_quantity"`
	Scaled          bool    `json:"scaled"`
	ScaledValue     float64 `json:"-"`
	Preparation     string  `json:"preparation,omitempty"`
}

// RecipeData mirrors the domain_data JSON structure for recipe artifacts.
type RecipeData struct {
	Domain      string       `json:"domain"`
	Title       string       `json:"title"`
	Servings    *int         `json:"servings"`
	Timing      TimingData   `json:"timing"`
	Cuisine     string       `json:"cuisine"`
	Difficulty  string       `json:"difficulty"`
	DietaryTags []string     `json:"dietary_tags"`
	Ingredients []Ingredient `json:"ingredients"`
	Steps       []Step       `json:"steps"`
}

// TimingData holds recipe timing information.
type TimingData struct {
	Prep  string `json:"prep"`
	Cook  string `json:"cook"`
	Total string `json:"total"`
}
