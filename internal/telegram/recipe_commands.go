package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/smackerel/smackerel/internal/recipe"
)

// Regex patterns for serving scaler triggers (UX-1.1).
var (
	scaleServingsRe   = regexp.MustCompile(`(?i)^(\d+)\s+servings?$`)
	scaleForRe        = regexp.MustCompile(`(?i)^for\s+(\d+)$`)
	scaleToRe         = regexp.MustCompile(`(?i)^scale\s+to\s+(\d+)$`)
	scalePeopleRe     = regexp.MustCompile(`(?i)^(\d+)\s+people$`)
	cookBareRe        = regexp.MustCompile(`(?i)^cook$`)
	cookNameRe        = regexp.MustCompile(`(?i)^cook\s+(.+?)$`)
	cookNameServRe    = regexp.MustCompile(`(?i)^cook\s+(.+?)\s+for\s+(\d+)\s+servings?$`)
	cookNavNextRe     = regexp.MustCompile(`(?i)^(next|n)$`)
	cookNavBackRe     = regexp.MustCompile(`(?i)^(back|b|prev|previous)$`)
	cookNavIngrRe     = regexp.MustCompile(`(?i)^(ingredients?|ing|i)$`)
	cookNavDoneRe     = regexp.MustCompile(`(?i)^(done|d|stop|exit)$`)
	cookNavJumpRe     = regexp.MustCompile(`^(\d+)$`)
	cookConfirmYesRe  = regexp.MustCompile(`(?i)^(yes|y)$`)
	cookConfirmNoRe   = regexp.MustCompile(`(?i)^(no|n)$`)
)

// parseScaleTrigger checks if text matches a serving scaler pattern.
// Returns the requested servings count, or 0 if no match.
func parseScaleTrigger(text string) int {
	text = strings.TrimSpace(text)

	for _, re := range []*regexp.Regexp{scaleServingsRe, scaleForRe, scaleToRe, scalePeopleRe} {
		if m := re.FindStringSubmatch(text); len(m) >= 2 {
			n, err := strconv.Atoi(m[1])
			if err == nil && n > 0 {
				return n
			}
		}
	}
	return 0
}

// parseCookTrigger checks if text matches a cook command pattern.
// Returns (recipeName, servings, matched).
// For bare "cook", recipeName is empty. servings is 0 if not specified.
func parseCookTrigger(text string) (string, int, bool) {
	text = strings.TrimSpace(text)

	// "cook {name} for {N} servings" — must check before cookNameRe
	if m := cookNameServRe.FindStringSubmatch(text); len(m) >= 3 {
		n, err := strconv.Atoi(m[2])
		if err == nil && n > 0 {
			return strings.TrimSpace(m[1]), n, true
		}
	}

	// "cook" bare
	if cookBareRe.MatchString(text) {
		return "", 0, true
	}

	// "cook {name}"
	if m := cookNameRe.FindStringSubmatch(text); len(m) >= 2 {
		return strings.TrimSpace(m[1]), 0, true
	}

	return "", 0, false
}

// parseCookNavigation checks if text is a cook navigation command.
// Returns the command type: "next", "back", "ingredients", "done", "jump:{N}", or "".
func parseCookNavigation(text string) string {
	text = strings.TrimSpace(text)

	if cookNavNextRe.MatchString(text) {
		return "next"
	}
	if cookNavBackRe.MatchString(text) {
		return "back"
	}
	if cookNavIngrRe.MatchString(text) {
		return "ingredients"
	}
	if cookNavDoneRe.MatchString(text) {
		return "done"
	}
	if m := cookNavJumpRe.FindStringSubmatch(text); len(m) >= 2 {
		return "jump:" + m[1]
	}
	return ""
}

// handleScaleTrigger handles a "{N} servings" type message.
func (b *Bot) handleScaleTrigger(ctx context.Context, chatID int64, requestedServings int) {
	// Resolve the most recently displayed recipe in this chat.
	// Use the recent API to find a recipe artifact.
	recipeData, artifactID, err := b.resolveRecentRecipe(ctx)
	if err != nil {
		slog.Warn("scale trigger: failed to resolve recent recipe", "error", err)
		b.reply(chatID, "? Which recipe? Send a recipe link or search with /find.")
		return
	}

	if recipeData.Servings == nil {
		b.reply(chatID, "? This recipe doesn't specify a base serving count. I can't scale without a baseline.")
		return
	}

	originalServings := *recipeData.Servings

	if requestedServings == originalServings {
		b.reply(chatID, fmt.Sprintf("> This recipe is already for %d servings.", originalServings))
		return
	}

	scaled := recipe.ScaleIngredients(recipeData.Ingredients, originalServings, requestedServings)
	if scaled == nil {
		b.reply(chatID, "? Unable to scale this recipe.")
		return
	}

	response := formatScaledResponse(recipeData.Title, originalServings, requestedServings, scaled)
	_ = artifactID // available for future linking
	b.reply(chatID, response)
}

// formatScaledResponse builds the scaled ingredient response per UX-1.2.
func formatScaledResponse(title string, originalServings, requestedServings int, scaled []recipe.ScaledIngredient) string {
	factor := float64(requestedServings) / float64(originalServings)

	var lines []string
	lines = append(lines, fmt.Sprintf("# %s — %d servings", title, requestedServings))

	factorStr := formatScaleFactor(factor)
	lines = append(lines, fmt.Sprintf("~ Scaled from %d to %d servings (%sx)", originalServings, requestedServings, factorStr))
	lines = append(lines, "")

	for _, ing := range scaled {
		if !ing.Scaled {
			// Unparseable quantity
			qtyPart := ing.Quantity
			if qtyPart == "" {
				qtyPart = ing.Name
				lines = append(lines, fmt.Sprintf("- %s (unscaled)", qtyPart))
			} else {
				unitPart := ""
				if ing.Unit != "" {
					unitPart = " " + ing.Unit
				}
				lines = append(lines, fmt.Sprintf("- %s%s %s (unscaled)", qtyPart, unitPart, ing.Name))
			}
		} else {
			unitPart := ""
			if ing.Unit != "" {
				unitPart = ing.Unit + " "
			}
			lines = append(lines, fmt.Sprintf("- %s%s%s", ing.DisplayQuantity, unitPart, ing.Name))
		}
	}

	return strings.Join(lines, "\n")
}

// formatScaleFactor formats a float factor for display (e.g., 2.0 → "2", 1.5 → "1.5").
func formatScaleFactor(factor float64) string {
	if factor == float64(int(factor)) {
		return fmt.Sprintf("%d", int(factor))
	}
	return fmt.Sprintf("%.1f", factor)
}

// resolveRecentRecipe fetches the most recently displayed recipe from the API.
func (b *Bot) resolveRecentRecipe(ctx context.Context) (*recipe.RecipeData, string, error) {
	body, err := b.apiGet(ctx, "/api/recent?limit=10")
	if err != nil {
		return nil, "", fmt.Errorf("fetch recent: %w", err)
	}

	var items []struct {
		ID         string          `json:"id"`
		DomainData json.RawMessage `json:"domain_data"`
	}
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, "", fmt.Errorf("parse recent: %w", err)
	}

	for _, item := range items {
		if len(item.DomainData) == 0 {
			continue
		}
		var rd recipe.RecipeData
		if err := json.Unmarshal(item.DomainData, &rd); err != nil {
			continue
		}
		if rd.Domain == "recipe" {
			// Convert ingredients from domain_data format
			return &rd, item.ID, nil
		}
	}

	return nil, "", fmt.Errorf("no recent recipe found")
}

// handleCookEntry starts a cook mode session for a recipe.
func (b *Bot) handleCookEntry(ctx context.Context, chatID int64, recipeName string, servings int) {
	if b.cookSessions == nil {
		b.reply(chatID, "? Cook mode is not available.")
		return
	}

	// Check for pending replacement confirmation
	if session := b.cookSessions.Get(chatID); session != nil && session.PendingReplacement != "" {
		// Already in replacement flow — this is a new cook command, treat as a new replacement
		session.PendingReplacement = ""
	}

	var rd *recipe.RecipeData
	var artifactID string
	var err error

	if recipeName == "" {
		// Bare "cook" — use most recently displayed recipe
		rd, artifactID, err = b.resolveRecentRecipe(ctx)
		if err != nil {
			b.reply(chatID, "? Which recipe? Send a name or search with /find.")
			return
		}
	} else {
		// Search for recipe by name
		rd, artifactID, err = b.resolveRecipeByName(ctx, recipeName)
		if err != nil {
			b.reply(chatID, fmt.Sprintf("? I don't have a recipe called \"%s\". Try /find %s to search.", recipeName, recipeName))
			return
		}
	}

	// Check for existing session — prompt replacement
	if existing := b.cookSessions.Get(chatID); existing != nil {
		existing.PendingReplacement = artifactID
		existing.PendingRecipeData = rd
		existing.PendingServings = servings
		existing.PendingRecipeName = rd.Title
		b.reply(chatID, fmt.Sprintf("? You're cooking %s (step %d of %d). Switch to %s?\n\nReply: yes · no",
			existing.RecipeTitle, existing.CurrentStep, existing.TotalSteps, rd.Title))
		return
	}

	b.startCookSession(chatID, rd, artifactID, servings)
}

// startCookSession creates a new cook session and displays step 1.
func (b *Bot) startCookSession(chatID int64, rd *recipe.RecipeData, artifactID string, servings int) {
	if len(rd.Steps) == 0 {
		// No steps — show ingredient list fallback
		b.reply(chatID, formatNoStepsFallback(rd))
		return
	}

	scaleFactor := 1.0
	originalServings := 0
	scaledServings := 0
	if servings > 0 && rd.Servings != nil && *rd.Servings > 0 {
		originalServings = *rd.Servings
		scaledServings = servings
		scaleFactor = float64(servings) / float64(*rd.Servings)
	} else if servings > 0 && (rd.Servings == nil || *rd.Servings <= 0) {
		b.reply(chatID, "? This recipe doesn't specify a base serving count. Starting cook mode without scaling.")
	}

	session := &CookSession{
		RecipeArtifactID: artifactID,
		RecipeTitle:      rd.Title,
		Steps:            rd.Steps,
		Ingredients:      rd.Ingredients,
		CurrentStep:      1,
		TotalSteps:       len(rd.Steps),
		ScaleFactor:      scaleFactor,
		OriginalServings: originalServings,
		ScaledServings:   scaledServings,
	}

	b.cookSessions.Create(chatID, session)

	// Display step 1
	b.reply(chatID, FormatCookStep(session))
}

// formatNoStepsFallback formats the response for a recipe with no steps.
func formatNoStepsFallback(rd *recipe.RecipeData) string {
	var lines []string
	lines = append(lines, "> This recipe has no steps to walk through.")
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("# %s — Ingredients", rd.Title))

	for _, ing := range rd.Ingredients {
		qty := ""
		if ing.Quantity != "" {
			qty = ing.Quantity
			if ing.Unit != "" {
				qty += " " + ing.Unit
			}
			qty += " "
		}
		lines = append(lines, fmt.Sprintf("- %s%s", qty, ing.Name))
	}

	return strings.Join(lines, "\n")
}

// handleCookNavigation processes a navigation command during an active cook session.
func (b *Bot) handleCookNavigation(ctx context.Context, chatID int64, nav string) {
	session := b.cookSessions.Get(chatID)
	if session == nil {
		b.reply(chatID, "? No active cook session. Send \"cook {recipe name}\" to start one.")
		return
	}

	// Handle pending replacement confirmation
	if session.PendingReplacement != "" {
		// Not a yes/no answer — clear pending and process as normal navigation
		session.PendingReplacement = ""
		session.PendingRecipeData = nil
		session.PendingServings = 0
		session.PendingRecipeName = ""
	}

	// Touch the session
	b.cookSessions.Touch(chatID)

	switch {
	case nav == "next":
		if session.CurrentStep >= session.TotalSteps {
			b.reply(chatID, "> That was the last step. Reply \"done\" when finished.")
			return
		}
		session.CurrentStep++
		b.reply(chatID, FormatCookStep(session))

	case nav == "back":
		if session.CurrentStep <= 1 {
			b.reply(chatID, "> Already at the first step.")
			return
		}
		session.CurrentStep--
		b.reply(chatID, FormatCookStep(session))

	case nav == "ingredients":
		b.reply(chatID, FormatCookIngredients(session))

	case nav == "done":
		b.cookSessions.Delete(chatID)
		b.reply(chatID, ". Cook session ended. Enjoy your meal.")

	case strings.HasPrefix(nav, "jump:"):
		numStr := strings.TrimPrefix(nav, "jump:")
		num, err := strconv.Atoi(numStr)
		if err != nil || num < 1 || num > session.TotalSteps {
			b.reply(chatID, fmt.Sprintf("? This recipe has %d steps. Pick a number from 1 to %d.", session.TotalSteps, session.TotalSteps))
			return
		}
		session.CurrentStep = num
		b.reply(chatID, FormatCookStep(session))
	}
}

// handleCookReplacement handles yes/no response to session replacement prompt.
func (b *Bot) handleCookReplacement(ctx context.Context, chatID int64, confirm bool) {
	session := b.cookSessions.Get(chatID)
	if session == nil || session.PendingReplacement == "" {
		return
	}

	if confirm {
		rd := session.PendingRecipeData
		artifactID := session.PendingReplacement
		servings := session.PendingServings
		b.cookSessions.Delete(chatID)
		b.startCookSession(chatID, rd, artifactID, servings)
	} else {
		title := session.RecipeTitle
		step := session.CurrentStep
		total := session.TotalSteps
		session.PendingReplacement = ""
		session.PendingRecipeData = nil
		session.PendingServings = 0
		session.PendingRecipeName = ""
		b.reply(chatID, fmt.Sprintf("> Continuing with %s. You're on step %d of %d.", title, step, total))
	}
}

// resolveRecipeByName searches for a recipe by name via the API.
func (b *Bot) resolveRecipeByName(ctx context.Context, name string) (*recipe.RecipeData, string, error) {
	// Truncate name for safety
	if len(name) > 200 {
		name = name[:200]
	}

	body, err := b.apiPost(ctx, "/api/search", map[string]string{"query": name + " recipe"})
	if err != nil {
		return nil, "", fmt.Errorf("search recipe: %w", err)
	}

	var results struct {
		Results []struct {
			ID         string          `json:"id"`
			DomainData json.RawMessage `json:"domain_data"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &results); err != nil {
		return nil, "", fmt.Errorf("parse search results: %w", err)
	}

	for _, item := range results.Results {
		if len(item.DomainData) == 0 {
			continue
		}
		var rd recipe.RecipeData
		if err := json.Unmarshal(item.DomainData, &rd); err != nil {
			continue
		}
		if rd.Domain == "recipe" && strings.Contains(strings.ToLower(rd.Title), strings.ToLower(name)) {
			return &rd, item.ID, nil
		}
	}

	return nil, "", fmt.Errorf("no recipe found matching %q", name)
}

// apiGet performs an authenticated GET request to the core API.
func (b *Bot) apiGet(ctx context.Context, path string) ([]byte, error) {
	url := b.baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if b.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+b.authToken)
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API GET %s: %w", path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB limit
	if err != nil {
		return nil, fmt.Errorf("read API response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API GET %s returned %d", path, resp.StatusCode)
	}
	return body, nil
}

// apiPost performs an authenticated POST request to the core API.
func (b *Bot) apiPost(ctx context.Context, path string, payload interface{}) ([]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	url := b.baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if b.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+b.authToken)
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API POST %s: %w", path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB limit
	if err != nil {
		return nil, fmt.Errorf("read API response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API POST %s returned %d", path, resp.StatusCode)
	}
	return body, nil
}
