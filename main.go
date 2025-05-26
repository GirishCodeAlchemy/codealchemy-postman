package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/PaesslerAG/jsonpath"
)

// Collection and Workspace structures

type APIRequest struct {
	Name    string            `json:"name"`
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}

type Collection struct {
	Name     string       `json:"name"`
	Requests []APIRequest `json:"requests"`
}

type Workspace struct {
	Name        string       `json:"name"`
	Collections []Collection `json:"collections"`
}

// Local storage helpers
func getStoragePath() string {
	dir, _ := os.UserHomeDir()
	return filepath.Join(dir, ".postman-go-workspaces.json")
}

func loadWorkspaces() ([]Workspace, error) {
	path := getStoragePath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return []Workspace{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var workspaces []Workspace
	err = json.Unmarshal(data, &workspaces)
	return workspaces, err
}

func saveWorkspaces(workspaces []Workspace) error {
	data, err := json.MarshalIndent(workspaces, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(getStoragePath(), data, 0644)
}

func parseHeaders(headerStr string) http.Header {
	headers := http.Header{}
	lines := strings.Split(headerStr, "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			headers.Add(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
		}
	}
	return headers
}

// Format size in bytes, KB, or MB
func formatSize(size int) string {
	if size < 1024 {
		return fmt.Sprintf("%d bytes", size)
	} else if size < 1024*1024 {
		return fmt.Sprintf("%.2f KB", float64(size)/1024.0)
	}
	return fmt.Sprintf("%.2f MB", float64(size)/(1024.0*1024.0))
}

func main() {
	a := app.New()
	w := a.NewWindow("codealchemyman)")

	// HTTP method dropdown
	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"}
	methodSelect := widget.NewSelect(methods, nil)
	methodSelect.SetSelected("GET")

	// URL entry
	urlEntry := widget.NewEntry()
	urlEntry.SetPlaceHolder("Enter request URL...")

	// Headers and body
	headersEntry := widget.NewMultiLineEntry()
	headersEntry.SetPlaceHolder("Headers (key: value, one per line)")
	bodyEntry := widget.NewMultiLineEntry()
	bodyEntry.SetPlaceHolder("Request body (JSON, form, etc.)")

	// Send button
	sendBtn := widget.NewButton("Send", func() {})

	// Response tabs
	// jsonResponse := widget.NewMultiLineEntry()
	// jsonResponse.SetPlaceHolder("JSON response will appear here...")
	// jsonResponse.SetMinRowsVisible(60)
	// jsonResponse.Wrapping = fyne.TextWrapBreak
	// jsonResponse.Disable()
	jsonResponse := widget.NewMultiLineEntry()
	jsonResponse.SetPlaceHolder("JSON response will appear here...")
	jsonResponse.SetMinRowsVisible(30)
	jsonResponse.Wrapping = fyne.TextWrapBreak // Keep for multi-line JSON
	jsonResponse.Enable()                      // Ensure it is enabled for rendering
	jsonResponseScroller := container.NewVScroll(jsonResponse)
	jsonResponseScroller.SetMinSize(fyne.NewSize(1000, 600))

	// Add response status, time, size display, and search/copy controls
	responseMeta := widget.NewLabel("") // Will be set after each request
	statusColor := canvas.NewRectangle(&color.NRGBA{0, 0, 0, 255})
	statusColor.SetMinSize(fyne.NewSize(18, 18))
	responseStatus := widget.NewLabel("")
	responseStatusContainer := container.NewHBox(statusColor, responseStatus, layout.NewSpacer(), responseMeta)

	// Enhanced search functionality with highlighting and dynamic sizing
	searchEntry := widget.NewEntry()
	searchEntry.SetPlaceHolder("Find in response...")
	searchEntry.Resize(fyne.NewSize(200, searchEntry.MinSize().Height)) // Initial size

	// Use icon buttons for search and copy (Postman style)
	searchIcon := widget.NewButtonWithIcon("", theme.SearchIcon(), nil)
	copyIcon := widget.NewButtonWithIcon("", theme.ContentCopyIcon(), nil)
	prevIcon := widget.NewButtonWithIcon("", theme.NavigateBackIcon(), nil)
	nextIcon := widget.NewButtonWithIcon("", theme.NavigateNextIcon(), nil)
	clearIcon := widget.NewButtonWithIcon("", theme.CancelIcon(), nil)

	// Variables to track search state
	var currentSearchQuery string
	var searchResults []int // Store all match positions
	var currentMatchIndex int = -1
	var originalText string // Store original text without highlighting

	// Update button states and match count
	updateSearchNav := func() {
		if len(searchResults) > 0 && currentMatchIndex >= 0 {
			prevIcon.Enable()
			nextIcon.Enable()
			clearIcon.Enable()
		} else {
			prevIcon.Disable()
			nextIcon.Disable()
			clearIcon.Disable()
		}
	}

	// Highlight function (same as before, but always highlights all matches, and current is styled differently)
	highlightText := func(text, query string, currentIdx int) string {
		if query == "" {
			return text
		}
		searchResults = []int{}
		lowerText := strings.ToLower(text)
		lowerQuery := strings.ToLower(query)
		startPos := 0
		for {
			idx := strings.Index(lowerText[startPos:], lowerQuery)
			if idx == -1 {
				break
			}
			absoluteIdx := startPos + idx
			searchResults = append(searchResults, absoluteIdx)
			startPos = absoluteIdx + len(query)
		}
		if len(searchResults) == 0 {
			return text
		}
		result := text
		offset := 0
		for i, matchPos := range searchResults {
			start := matchPos + offset
			end := start + len(query)
			if end <= len(result) {
				var highlighted string
				if i == currentIdx {
					highlighted = "【" + result[start:end] + "】" // Current match
				} else {
					highlighted = "〔" + result[start:end] + "〕" // Other matches
				}
				result = result[:start] + highlighted + result[end:]
				offset += len(highlighted) - len(query)
			}
		}
		return result
	}

	// Navigation logic
	navigateToMatch := func(matchIndex int) {
		if len(searchResults) == 0 || matchIndex < 0 || matchIndex >= len(searchResults) {
			return
		}
		currentMatchIndex = matchIndex
		if originalText != "" && currentSearchQuery != "" {
			highlightedText := highlightText(originalText, currentSearchQuery, currentMatchIndex)
			jsonResponse.SetText(highlightedText)
		}
		// Scroll to match (approximate)
		matchPos := searchResults[matchIndex]
		text := originalText
		if text == "" {
			text = jsonResponse.Text
		}
		before := text[:matchPos]
		row := strings.Count(before, "\n")
		jsonResponse.CursorRow = row
		jsonResponse.CursorColumn = 0
		jsonResponse.Refresh()
		updateSearchNav()
	}

	// Search action
	searchAction := func() {
		query := strings.TrimSpace(searchEntry.Text)
		if query == "" {
			if originalText != "" {
				jsonResponse.SetText(originalText)
				originalText = ""
			}
			currentSearchQuery = ""
			searchResults = []int{}
			currentMatchIndex = -1
			updateSearchNav()
			return
		}
		if currentSearchQuery != query {
			if originalText == "" {
				originalText = jsonResponse.Text
			}
			currentSearchQuery = query
			_ = highlightText(originalText, query, 0) // update searchResults
			if len(searchResults) > 0 {
				currentMatchIndex = 0
				navigateToMatch(0)
			} else {
				jsonResponse.SetText(originalText)
				currentMatchIndex = -1
			}
			updateSearchNav()
		} else if len(searchResults) > 0 {
			// Next match
			nextIdx := (currentMatchIndex + 1) % len(searchResults)
			navigateToMatch(nextIdx)
		}
	}

	// Icon button callbacks
	searchIcon.OnTapped = searchAction
	searchEntry.OnSubmitted = func(_ string) { searchAction() }
	prevIcon.OnTapped = func() {
		if len(searchResults) == 0 {
			return
		}
		prevIdx := currentMatchIndex - 1
		if prevIdx < 0 {
			prevIdx = len(searchResults) - 1
		}
		navigateToMatch(prevIdx)
	}
	nextIcon.OnTapped = func() {
		if len(searchResults) == 0 {
			return
		}
		nextIdx := (currentMatchIndex + 1) % len(searchResults)
		navigateToMatch(nextIdx)
	}
	clearIcon.OnTapped = func() {
		searchEntry.SetText("")
		if originalText != "" {
			jsonResponse.SetText(originalText)
			originalText = ""
		}
		currentSearchQuery = ""
		searchResults = []int{}
		currentMatchIndex = -1
		updateSearchNav()
	}
	copyIcon.OnTapped = func() {
		textToCopy := originalText
		if textToCopy == "" {
			textToCopy = jsonResponse.Text
		}
		w.Clipboard().SetContent(textToCopy)
		dialog.ShowInformation("Copied", "Response copied to clipboard!", w)
	}

	// Overlay search bar styled like Postman (floating, top right)
	searchBarOverlay := container.NewHBox(
		container.NewHBox(
			searchEntry,
			searchIcon,
			prevIcon,
			nextIcon,
			clearIcon,
			copyIcon,
		),
	)
	searchBarOverlayBG := container.NewVBox(
		canvas.NewRectangle(color.NRGBA{240, 240, 240, 220}),
		container.NewHBox(layout.NewSpacer(), searchBarOverlay),
	)
	searchBarOverlayBG.Objects[0].Resize(fyne.NewSize(420, 44)) // Set overlay background size

	// Place overlay above response area, top right
	jsonResponseWithOverlay := container.NewStack(
		jsonResponseScroller,
		container.NewVBox(
			container.NewHBox(layout.NewSpacer(), searchBarOverlayBG),
		),
	)

	// Add response status and headers display
	// statusLabel := widget.NewLabel("")
	headersBox := widget.NewMultiLineEntry()
	headersBox.SetPlaceHolder("Response headers will appear here...")

	// UI for workspaces/collections
	workspaces, _ := loadWorkspaces()

	// Track selected collection index
	var selectedCollectionIdx int = -1

	// Forward declare UI elements that will be referenced in functions
	var workspaceSelect *widget.Select
	var collectionSelect *widget.Select
	var requestList *widget.List

	// Workspace management functions
	createNewWorkspace := func() {
		entry := widget.NewEntry()
		form := dialog.NewForm("New Workspace", "Create", "Cancel", []*widget.FormItem{
			widget.NewFormItem("Workspace Name", entry),
		}, func(ok bool) {
			if !ok || entry.Text == "" {
				return
			}
			workspaces = append(workspaces, Workspace{Name: entry.Text, Collections: []Collection{}})
			err := saveWorkspaces(workspaces)
			if err == nil {
				workspaceNames := []string{"+ New Workspace"}
				for _, ws := range workspaces {
					workspaceNames = append(workspaceNames, ws.Name)
				}
				workspaceSelect.Options = workspaceNames
				workspaceSelect.SetSelected(entry.Text)
				collectionSelect.Options = []string{"+ New Collection"}
				collectionSelect.SetSelected("")
				requestList.Refresh()
			}
		}, w)
		form.Show()
	}

	// Collection management functions
	createNewCollection := func() {
		if workspaceSelect.Selected == "" || workspaceSelect.Selected == "+ New Workspace" {
			dialog.ShowInformation("No Workspace", "Select a workspace first.", w)
			return
		}
		entry := widget.NewEntry()
		form := dialog.NewForm("New Collection", "Create", "Cancel", []*widget.FormItem{
			widget.NewFormItem("Collection Name", entry),
		}, func(ok bool) {
			if !ok || entry.Text == "" {
				return
			}
			for i, ws := range workspaces {
				if ws.Name == workspaceSelect.Selected {
					workspaces[i].Collections = append(workspaces[i].Collections, Collection{Name: entry.Text})
					err := saveWorkspaces(workspaces)
					if err == nil {
						// Update collection dropdown options
						collectionOptions := []string{"+ New Collection"}
						for _, col := range workspaces[i].Collections {
							collectionOptions = append(collectionOptions, col.Name)
						}
						collectionSelect.Options = collectionOptions
						collectionSelect.SetSelected(entry.Text)
						selectedCollectionIdx = len(workspaces[i].Collections) - 1
						requestList.Refresh()
					}
				}
			}
		}, w)
		form.Show()
	}

	// Request management functions
	editRequestName := func(reqIdx int) {
		if workspaceSelect.Selected == "" || workspaceSelect.Selected == "+ New Workspace" || selectedCollectionIdx < 0 {
			return
		}
		wsIdx := -1
		for i, ws := range workspaces {
			if ws.Name == workspaceSelect.Selected {
				wsIdx = i
				break
			}
		}
		if wsIdx == -1 || selectedCollectionIdx >= len(workspaces[wsIdx].Collections) {
			return
		}

		coll := &workspaces[wsIdx].Collections[selectedCollectionIdx]
		if reqIdx >= len(coll.Requests) {
			return
		}

		entry := widget.NewEntry()
		entry.SetText(coll.Requests[reqIdx].Name)
		form := dialog.NewForm("Edit Request Name", "Save", "Cancel", []*widget.FormItem{
			widget.NewFormItem("Request Name", entry),
		}, func(ok bool) {
			if !ok || entry.Text == "" {
				return
			}
			coll.Requests[reqIdx].Name = entry.Text
			err := saveWorkspaces(workspaces)
			if err == nil {
				requestList.Refresh()
			}
		}, w)
		form.Show()
	}

	deleteRequest := func(reqIdx int) {
		if workspaceSelect.Selected == "" || workspaceSelect.Selected == "+ New Workspace" || selectedCollectionIdx < 0 {
			return
		}
		wsIdx := -1
		for i, ws := range workspaces {
			if ws.Name == workspaceSelect.Selected {
				wsIdx = i
				break
			}
		}
		if wsIdx == -1 || selectedCollectionIdx >= len(workspaces[wsIdx].Collections) {
			return
		}

		coll := &workspaces[wsIdx].Collections[selectedCollectionIdx]
		if reqIdx >= len(coll.Requests) {
			return
		}

		reqName := coll.Requests[reqIdx].Name
		dialog.ShowConfirm("Delete Request",
			fmt.Sprintf("Are you sure you want to delete the request '%s'?", reqName),
			func(confirmed bool) {
				if confirmed {
					// Remove the request from the slice
					coll.Requests = append(coll.Requests[:reqIdx], coll.Requests[reqIdx+1:]...)
					err := saveWorkspaces(workspaces)
					if err == nil {
						requestList.Refresh()
					}
				}
			}, w)
	}

	// Create workspace dropdown
	workspaceNames := []string{"+ New Workspace"}
	for _, ws := range workspaces {
		workspaceNames = append(workspaceNames, ws.Name)
	}
	workspaceSelect = widget.NewSelect(workspaceNames, nil)

	// Collection dropdown
	collectionSelect = widget.NewSelect([]string{"+ New Collection"}, nil)

	// Request list for selected collection with edit/delete functionality
	requestList = widget.NewList(
		func() int {
			if workspaceSelect.Selected == "" || workspaceSelect.Selected == "+ New Workspace" || selectedCollectionIdx < 0 {
				return 0
			}
			for _, ws := range workspaces {
				if ws.Name == workspaceSelect.Selected {
					if selectedCollectionIdx < len(ws.Collections) {
						return len(ws.Collections[selectedCollectionIdx].Requests)
					}
				}
			}
			return 0
		},
		func() fyne.CanvasObject {
			// Create a container with request name, edit button, and delete button
			nameLabel := widget.NewLabel("")
			editBtn := widget.NewButtonWithIcon("", theme.DocumentCreateIcon(), nil)
			deleteBtn := widget.NewButtonWithIcon("", theme.DeleteIcon(), nil)

			editBtn.Resize(fyne.NewSize(24, 24))
			deleteBtn.Resize(fyne.NewSize(24, 24))

			return container.NewBorder(nil, nil, nil,
				container.NewHBox(editBtn, deleteBtn),
				nameLabel)
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			// Cast to fyne.Container instead of container.Border
			containerObj := o.(*fyne.Container)

			// In a border container, the main object is at index 0, and the trailing object (buttons) is at index 1
			nameLabel := containerObj.Objects[0].(*widget.Label)
			buttonContainer := containerObj.Objects[1].(*fyne.Container)
			editBtn := buttonContainer.Objects[0].(*widget.Button)
			deleteBtn := buttonContainer.Objects[1].(*widget.Button)

			for _, ws := range workspaces {
				if ws.Name == workspaceSelect.Selected {
					if selectedCollectionIdx < len(ws.Collections) {
						requests := ws.Collections[selectedCollectionIdx].Requests
						if i < len(requests) {
							nameLabel.SetText(requests[i].Name)

							// Set up edit button callback (capture i in closure)
							reqIdx := i
							editBtn.OnTapped = func() {
								editRequestName(reqIdx)
							}

							// Set up delete button callback (capture i in closure)
							deleteBtn.OnTapped = func() {
								deleteRequest(reqIdx)
							}
						}
					}
				}
			}
		},
	)

	// Set up click handler to load request
	requestList.OnSelected = func(id int) {
		// Load the request into the form
		for _, ws := range workspaces {
			if ws.Name == workspaceSelect.Selected {
				if selectedCollectionIdx < len(ws.Collections) {
					requests := ws.Collections[selectedCollectionIdx].Requests
					if id < len(requests) {
						r := requests[id]
						methodSelect.SetSelected(r.Method)
						urlEntry.SetText(r.URL)
						headersEntry.SetText("")
						for k, v := range r.Headers {
							headersEntry.SetText(headersEntry.Text + k + ": " + v + "\n")
						}
						bodyEntry.SetText(r.Body)
					}
				}
			}
		}
	}

	// Set up workspace selection callback after all widgets are created
	workspaceSelect.OnChanged = func(selected string) {
		if selected == "+ New Workspace" {
			createNewWorkspace()
			return
		}
		// Update collection dropdown when workspace changes
		selectedCollectionIdx = -1
		collectionOptions := []string{"+ New Collection"}
		for _, ws := range workspaces {
			if ws.Name == selected {
				for _, col := range ws.Collections {
					collectionOptions = append(collectionOptions, col.Name)
				}
				break
			}
		}
		collectionSelect.Options = collectionOptions
		collectionSelect.SetSelected("")
		requestList.Refresh()
	}

	// Set up collection selection callback
	collectionSelect.OnChanged = func(selected string) {
		if selected == "+ New Collection" {
			createNewCollection()
			return
		}
		// Find the collection index
		selectedCollectionIdx = -1
		for _, ws := range workspaces {
			if ws.Name == workspaceSelect.Selected {
				for i, col := range ws.Collections {
					if col.Name == selected {
						selectedCollectionIdx = i
						break
					}
				}
				break
			}
		}
		requestList.Refresh()
	}

	if len(workspaceNames) > 1 {
		workspaceSelect.SetSelected(workspaceNames[1]) // Select first actual workspace, not "+ New Workspace"
	}

	// Initialize collection options for the selected workspace
	if workspaceSelect.Selected != "" && workspaceSelect.Selected != "+ New Workspace" {
		collectionOptions := []string{"+ New Collection"}
		for _, ws := range workspaces {
			if ws.Name == workspaceSelect.Selected {
				for _, col := range ws.Collections {
					collectionOptions = append(collectionOptions, col.Name)
				}
				break
			}
		}
		collectionSelect.Options = collectionOptions
	}

	// Flows canvas placeholder
	flowsLabel := widget.NewLabel("Flows canvas: Drag and chain API calls here (future)")

	// JSONata search UI
	jsonataEntry := widget.NewEntry()
	jsonataEntry.SetPlaceHolder("Enter JSONata expression (e.g. $.foo.bar)")
	jsonataOutput := widget.NewMultiLineEntry()
	jsonataOutput.SetPlaceHolder("JSONata output will appear here...")
	jsonataOutput.SetMinRowsVisible(30)
	jsonResponse.Wrapping = fyne.TextWrapBreak
	jsonataOutput.Enable()
	jsonataBtn := widget.NewButton("Apply JSONata", func() {
		expr := jsonataEntry.Text
		if expr == "" {
			dialog.ShowInformation("No Expression", "Please enter a JSONata expression.", w)
			return
		}
		var jsonData interface{}
		jsonText := originalText
		if jsonText == "" {
			jsonText = jsonResponse.Text
		}
		if err := json.Unmarshal([]byte(jsonText), &jsonData); err != nil {
			dialog.ShowError(fmt.Errorf("Invalid JSON: %v", err), w)
			return
		}
		res, err := jsonpath.Get(expr, jsonData)
		if err != nil {
			dialog.ShowError(fmt.Errorf("JSONata error: %v", err), w)
			return
		}
		resStr, _ := json.MarshalIndent(res, "", "  ")
		jsonataOutput.SetText(string(resStr))
		// Optionally, do not overwrite the main response box
		// jsonResponse.SetText(string(resStr))
		// Reset search state after applying JSONata
		originalText = jsonResponse.Text
		currentSearchQuery = ""
		searchResults = []int{}
		currentMatchIndex = -1
		updateSearchNav()
	})

	sendBtn.OnTapped = func() {
		method := methodSelect.Selected
		url := urlEntry.Text
		headers := parseHeaders(headersEntry.Text)
		body := bodyEntry.Text

		var req *http.Request
		var err error
		var reqSize int
		if method == "GET" || method == "DELETE" || method == "HEAD" || method == "OPTIONS" {
			req, err = http.NewRequest(method, url, nil)
			reqSize = 0
		} else {
			bodyBytes := []byte(body)
			req, err = http.NewRequest(method, url, bytes.NewBuffer(bodyBytes))
			reqSize = len(bodyBytes)
		}
		if err != nil {
			jsonResponse.SetText(fmt.Sprintf("Request error: %v", err))
			// statusLabel.SetText("")
			headersBox.SetText("")
			responseMeta.SetText("")
			// Reset search state on error
			originalText = ""
			currentSearchQuery = ""
			searchResults = []int{}
			currentMatchIndex = -1
			return
		}
		for k, v := range headers {
			req.Header[k] = v
		}
		client := &http.Client{}
		startTime := time.Now()
		resp, err := client.Do(req)
		elapsed := time.Since(startTime)
		if err != nil {
			jsonResponse.SetText(fmt.Sprintf("HTTP error: %v", err))
			// statusLabel.SetText("")
			headersBox.SetText("")
			responseMeta.SetText("")
			// Reset search state on error
			originalText = ""
			currentSearchQuery = ""
			searchResults = []int{}
			currentMatchIndex = -1
			return
		}
		defer resp.Body.Close()
		respBody, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			jsonResponse.SetText(fmt.Sprintf("Read error: %v", err))
			// statusLabel.SetText("")
			headersBox.SetText("")
			responseMeta.SetText("")
			// Reset search state on error
			originalText = ""
			currentSearchQuery = ""
			searchResults = []int{}
			currentMatchIndex = -1
			return
		}
		// Try to pretty-print JSON
		var prettyJSON bytes.Buffer
		if json.Valid(respBody) {
			err = json.Indent(&prettyJSON, respBody, "", "    ") // 4 spaces
			if err == nil {
				jsonResponse.SetText(prettyJSON.String())
			} else {
				jsonResponse.SetText(string(respBody))
			}
		} else {
			jsonResponse.SetText(string(respBody))
		}

		// Reset search state when new response comes in
		originalText = ""
		currentSearchQuery = ""
		searchResults = []int{}
		currentMatchIndex = -1
		// Status label (for headers panel)
		// statusLabel.SetText(fmt.Sprintf("Status: %d %s", resp.StatusCode, resp.Status))
		// Format response headers
		headersStr := ""
		for k, v := range resp.Header {
			headersStr += fmt.Sprintf("%s: %s\n", k, strings.Join(v, ", "))
		}
		headersBox.SetText(headersStr)
		// Set response meta info
		respSize := len(respBody)
		responseMeta.SetText(fmt.Sprintf("%d ms    Req: %s    Resp: %s",
			elapsed.Milliseconds(),
			formatSize(reqSize),
			formatSize(respSize),
		))
		// Status code indicator with emoji and text (no color/style)
		var statusText string
		switch {
		case resp.StatusCode >= 200 && resp.StatusCode < 300:
			statusColor.FillColor = color.NRGBA{0, 200, 0, 255} // Green
			statusText = fmt.Sprintf("✅ %d OK", resp.StatusCode)
		case resp.StatusCode >= 400:
			statusColor.FillColor = color.NRGBA{200, 0, 0, 255} // Red
			statusText = fmt.Sprintf("🔴 %d Error", resp.StatusCode)
		default:
			statusColor.FillColor = color.NRGBA{200, 200, 0, 255} // Yellow
			statusText = fmt.Sprintf("🟡 %d", resp.StatusCode)
		}
		statusColor.Refresh()
		responseStatus.SetText(statusText)
		responseStatus.Refresh()
	}

	// Add buttons for saving/loading requests and collections
	saveReqBtn := widget.NewButton("Save Request", func() {
		if workspaceSelect.Selected == "" || workspaceSelect.Selected == "+ New Workspace" {
			dialog.ShowInformation("No Workspace", "Please select a workspace.", w)
			return
		}
		if selectedCollectionIdx < 0 {
			dialog.ShowInformation("No Collection", "Please select a collection.", w)
			return
		}
		wsIdx := -1
		for i, ws := range workspaces {
			if ws.Name == workspaceSelect.Selected {
				wsIdx = i
				break
			}
		}
		if wsIdx == -1 {
			dialog.ShowError(fmt.Errorf("Workspace not found"), w)
			return
		}
		if selectedCollectionIdx >= len(workspaces[wsIdx].Collections) {
			dialog.ShowInformation("No Collection Selected", "Please select a collection.", w)
			return
		}
		colIdx := selectedCollectionIdx
		// Build request
		headersMap := map[string]string{}
		for k, v := range parseHeaders(headersEntry.Text) {
			headersMap[k] = strings.Join(v, ", ")
		}
		req := APIRequest{
			Name:    urlEntry.Text,
			Method:  methodSelect.Selected,
			URL:     urlEntry.Text,
			Headers: headersMap,
			Body:    bodyEntry.Text,
		}
		workspaces[wsIdx].Collections[colIdx].Requests = append(workspaces[wsIdx].Collections[colIdx].Requests, req)
		err := saveWorkspaces(workspaces)
		if err != nil {
			dialog.ShowError(err, w)
			return
		}
		requestList.Refresh()
		dialog.ShowInformation("Saved", "Request saved to collection.", w)
	})

	loadReqBtn := widget.NewButton("Load Request", func() {
		if workspaceSelect.Selected == "" || selectedCollectionIdx < 0 {
			dialog.ShowInformation("Select", "Select a workspace and collection.", w)
			return
		}
		wsIdx := -1
		colIdx := selectedCollectionIdx
		for i, ws := range workspaces {
			if ws.Name == workspaceSelect.Selected {
				wsIdx = i
				break
			}
		}
		if wsIdx == -1 || colIdx < 0 || colIdx >= len(workspaces[wsIdx].Collections) {
			dialog.ShowError(fmt.Errorf("Invalid selection"), w)
			return
		}
		coll := workspaces[wsIdx].Collections[colIdx]
		if len(coll.Requests) == 0 {
			dialog.ShowInformation("No Requests", "No requests in this collection.", w)
			return
		}
		// Show a dialog to pick a request
		reqNames := []string{}
		for _, r := range coll.Requests {
			reqNames = append(reqNames, r.Name)
		}
		pick := widget.NewSelect(reqNames, func(sel string) {
			for _, r := range coll.Requests {
				if r.Name == sel {
					methodSelect.SetSelected(r.Method)
					urlEntry.SetText(r.URL)
					headersEntry.SetText("")
					for k, v := range r.Headers {
						headersEntry.SetText(headersEntry.Text + k + ": " + v + "\n")
					}
					bodyEntry.SetText(r.Body)
				}
			}
		})
		d := dialog.NewCustom("Pick Request", "Close", pick, w)
		d.Show()
	})

	// Import/Export Dropdown Functions
	importPostmanJSON := func() {
		dialog.ShowFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil || reader == nil {
				return
			}
			defer reader.Close()
			var postman struct {
				Info struct{ Name string } `json:"info"`
				Item []struct {
					Name    string `json:"name"`
					Request struct {
						Method string      `json:"method"`
						URL    interface{} `json:"url"`
						Header []struct {
							Key   string `json:"key"`
							Value string `json:"value"`
						} `json:"header"`
						Body struct {
							Raw string `json:"raw"`
						} `json:"body"`
					} `json:"request"`
				} `json:"item"`
			}
			data, _ := ioutil.ReadAll(reader)
			err = json.Unmarshal(data, &postman)
			if err != nil {
				dialog.ShowError(fmt.Errorf("Invalid JSON: %v", err), w)
				return
			}
			if workspaceSelect.Selected == "" {
				dialog.ShowInformation("No Workspace", "Select a workspace first.", w)
				return
			}
			for i, ws := range workspaces {
				if ws.Name == workspaceSelect.Selected {
					col := Collection{Name: postman.Info.Name}
					for _, item := range postman.Item {
						headers := map[string]string{}
						for _, h := range item.Request.Header {
							headers[h.Key] = h.Value
						}
						urlStr := ""
						switch v := item.Request.URL.(type) {
						case string:
							urlStr = v
						case map[string]interface{}:
							if raw, ok := v["raw"].(string); ok {
								urlStr = raw
							}
						}
						col.Requests = append(col.Requests, APIRequest{
							Name:    item.Name,
							Method:  item.Request.Method,
							URL:     urlStr,
							Headers: headers,
							Body:    item.Request.Body.Raw,
						})
					}
					workspaces[i].Collections = append(workspaces[i].Collections, col)
					_ = saveWorkspaces(workspaces)
					// Update collection dropdown options
					collectionOptions := []string{"+ New Collection"}
					for _, col := range workspaces[i].Collections {
						collectionOptions = append(collectionOptions, col.Name)
					}
					collectionSelect.Options = collectionOptions
					collectionSelect.SetSelected(col.Name)
					selectedCollectionIdx = len(workspaces[i].Collections) - 1
					requestList.Refresh()
				}
			}
		}, w)
	}

	exportCollectionJSON := func() {
		if workspaceSelect.Selected == "" || selectedCollectionIdx < 0 {
			dialog.ShowInformation("Select", "Select a workspace and collection.", w)
			return
		}
		wsIdx := -1
		colIdx := selectedCollectionIdx
		for i, ws := range workspaces {
			if ws.Name == workspaceSelect.Selected {
				wsIdx = i
				break
			}
		}
		if wsIdx == -1 || colIdx < 0 || colIdx >= len(workspaces[wsIdx].Collections) {
			dialog.ShowError(fmt.Errorf("Invalid selection"), w)
			return
		}
		coll := workspaces[wsIdx].Collections[colIdx]
		// Convert to Postman v2.1 format
		postman := map[string]interface{}{
			"info": map[string]interface{}{
				"name":   coll.Name,
				"schema": "https://schema.getpostman.com/json/collection/v2.1.0/collection.json",
			},
			"item": []interface{}{},
		}
		for _, r := range coll.Requests {
			item := map[string]interface{}{
				"name": r.Name,
				"request": map[string]interface{}{
					"method": r.Method,
					"header": func() []interface{} {
						h := []interface{}{}
						for k, v := range r.Headers {
							h = append(h, map[string]interface{}{"key": k, "value": v})
						}
						return h
					}(),
					"url":  r.URL,
					"body": map[string]interface{}{"mode": "raw", "raw": r.Body},
				},
			}
			postman["item"] = append(postman["item"].([]interface{}), item)
		}
		data, _ := json.MarshalIndent(postman, "", "  ")
		dialog.ShowFileSave(func(writer fyne.URIWriteCloser, err error) {
			if err != nil || writer == nil {
				return
			}
			defer writer.Close()
			_, err = writer.Write(data)
			if err != nil {
				dialog.ShowError(fmt.Errorf("Write error: %v", err), w)
			}
		}, w)
	}

	// Import Dropdown
	importOptions := []string{"Postman Collection JSON"}
	var importSelect *widget.Select
	importSelect = widget.NewSelect(importOptions, func(selected string) {
		switch selected {
		case "Postman Collection JSON":
			importPostmanJSON()
		}
		// Reset selection after action
		go func() {
			time.Sleep(100 * time.Millisecond)
			importSelect.SetSelected("")
		}()
	})
	importSelect.PlaceHolder = "Import..."

	// Export Dropdown
	exportOptions := []string{"Collection as JSON"}
	var exportSelect *widget.Select
	exportSelect = widget.NewSelect(exportOptions, func(selected string) {
		switch selected {
		case "Collection as JSON":
			exportCollectionJSON()
		}
		// Reset selection after action
		go func() {
			time.Sleep(100 * time.Millisecond)
			exportSelect.SetSelected("")
		}()
	})
	exportSelect.PlaceHolder = "Export..."

	// --- UI Layout Improvements ---
	// Sidebar: vertical, with clear sectioning and spacing
	sidebar := container.NewVBox(
		// Top Row: Workspaces section with dropdown, and Import/Export dropdowns
		container.NewHBox(
			container.NewVBox(
				widget.NewLabelWithStyle("Workspaces", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
				workspaceSelect,
			),
			layout.NewSpacer(),
			container.NewVBox(
				importSelect,
				exportSelect,
			),
		),
		widget.NewSeparator(),
		// Collections section with dropdown
		widget.NewLabelWithStyle("Collections", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		collectionSelect,
		widget.NewSeparator(),
		// Requests section with scrollable list (limited to 10 items visible)
		widget.NewLabelWithStyle("Requests", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		func() *container.Scroll {
			scroll := container.NewVScroll(requestList)
			scroll.SetMinSize(fyne.NewSize(250, 300)) // Limit height to show ~10 items
			return scroll
		}(),
		widget.NewSeparator(),
		flowsLabel,
	)

	// Request Row: method, URL, Send button (URL entry with larger width and resizable)
	urlEntry.MultiLine = false
	urlEntry.Wrapping = fyne.TextWrapOff
	urlSplit := container.NewHSplit(methodSelect, urlEntry)
	urlSplit.Offset = 0.11 // Start with method select smaller

	// Increase the size of the Send button
	sendBtn.Importance = widget.HighImportance
	sendBtn.Resize(fyne.NewSize(400, 44)) // Wider and taller

	requestRow := container.NewBorder(nil, nil, nil, sendBtn, urlSplit)

	// Save/Load Row
	saveLoadRow := container.NewHBox(
		layout.NewSpacer(),
		saveReqBtn,
		loadReqBtn,
	)

	// Headers/Body Tabs
	headersTab := container.NewTabItem("Headers", headersEntry)
	bodyTab := container.NewTabItem("Body", bodyEntry)
	requestTabs := container.NewAppTabs(headersTab, bodyTab)
	requestTabs.SetTabLocation(container.TabLocationTop)

	// JSONata input row: make entry and button resizable
	jsonataSplit := container.NewHSplit(jsonataEntry, jsonataBtn)
	jsonataSplit.Offset = 0.8 // Entry gets most of the space
	jsonataTab := container.NewTabItem("JSONata", container.NewVBox(
		widget.NewLabelWithStyle("JSONata Query", fyne.TextAlignLeading, fyne.TextStyle{}),
		container.NewHSplit(jsonataEntry, jsonataBtn),
		widget.NewLabelWithStyle("Output", fyne.TextAlignLeading, fyne.TextStyle{}),
		jsonataOutput,
	))

	// Response tabs with status container
	jsonTabContent := container.NewVBox(
		responseStatusContainer,
		jsonResponseWithOverlay,
	)
	responseTabs := container.NewAppTabs(
		container.NewTabItem("JSON", jsonTabContent),
		container.NewTabItem("Preview", widget.NewLabel("Preview will appear here.")),
		container.NewTabItem("Visualize", widget.NewLabel("Visualization will appear here.")),
		jsonataTab,
	)
	responseTabs.SetTabLocation(container.TabLocationTop)

	// Response Section
	responseSection := container.NewVBox(
		widget.NewLabelWithStyle("Response", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		// container.NewHBox(statusLabel),
		headersBox,
		responseTabs,
	)

	// Main right pane: vertical, with clear separation
	rightPane := container.NewVBox(
		requestRow,
		saveLoadRow,
		requestTabs,
		widget.NewSeparator(),
		responseSection,
	)

	// Main layout: horizontal split, sidebar and right pane
	split := container.NewHSplit(
		container.NewVBox(sidebar),
		container.NewVScroll(rightPane),
	)
	split.Offset = 0.11 // Sidebar width smaller than right pane
	w.SetContent(split)
	w.Resize(fyne.NewSize(2000, 1200))
	urlEntry.Resize(fyne.NewSize(900, urlEntry.MinSize().Height)) // Set width after window is created
	w.ShowAndRun()
}
