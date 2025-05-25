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
	w := a.NewWindow("Postman-Go (Fyne)")

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
	// responseStatus := widget.NewLabel("") // Color-coded status code
	// Instead, use a colored rectangle and label for status
	statusColor := canvas.NewRectangle(&color.NRGBA{0, 0, 0, 255})
	statusColor.SetMinSize(fyne.NewSize(18, 18))
	responseStatus := widget.NewLabel("")
	responseStatusContainer := container.NewHBox(statusColor, responseStatus)

	searchEntry := widget.NewEntry()
	searchEntry.SetPlaceHolder("Search in response...")
	copyBtn := widget.NewButton("Copy", func() {
		w.Clipboard().SetContent(jsonResponse.Text)
		dialog.ShowInformation("Copied", "Response copied to clipboard!", w)
	})
	searchBtn := widget.NewButton("Find", func() {
		query := searchEntry.Text
		if query == "" {
			return
		}
		text := jsonResponse.Text
		idx := strings.Index(strings.ToLower(text), strings.ToLower(query))
		if idx >= 0 {
			before := text[:idx]
			row := strings.Count(before, "\n")
			col := idx - strings.LastIndex(before, "\n") - 1
			if strings.LastIndex(before, "\n") == -1 {
				col = idx
			}
			jsonResponse.CursorRow = row
			jsonResponse.CursorColumn = col
			jsonResponse.Refresh()
		} else {
			dialog.ShowInformation("Not found", "Text not found in response.", w)
		}
	})
	responseControls := container.NewHBox(
		responseStatusContainer,
		responseMeta,
		layout.NewSpacer(),
		searchEntry, searchBtn, copyBtn,
	)

	responseTabs := container.NewAppTabs(
		container.NewTabItem("JSON", container.NewVBox(responseControls, jsonResponseScroller)),
		container.NewTabItem("Preview", widget.NewLabel("Preview will appear here.")),
		container.NewTabItem("Visualize", widget.NewLabel("Visualization will appear here.")),
	)

	// Add response status and headers display
	// statusLabel := widget.NewLabel("")
	headersBox := widget.NewMultiLineEntry()
	headersBox.SetPlaceHolder("Response headers will appear here...")

	// UI for workspaces/collections
	workspaces, _ := loadWorkspaces()
	workspaceNames := []string{}
	for _, ws := range workspaces {
		workspaceNames = append(workspaceNames, ws.Name)
	}
	workspaceSelect := widget.NewSelect(workspaceNames, nil)
	if len(workspaceNames) > 0 {
		workspaceSelect.SetSelected(workspaceNames[0])
	}
	collectionList := widget.NewList(
		func() int {
			if workspaceSelect.Selected == "" {
				return 0
			}
			for _, ws := range workspaces {
				if ws.Name == workspaceSelect.Selected {
					return len(ws.Collections)
				}
			}
			return 0
		},
		func() fyne.CanvasObject {
			return widget.NewLabel("")
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			for _, ws := range workspaces {
				if ws.Name == workspaceSelect.Selected {
					if i < len(ws.Collections) {
						col := ws.Collections[i]
						label := o.(*widget.Label)
						label.SetText(col.Name)
					}
				}
			}
		},
	)

	// Track selected collection index
	var selectedCollectionIdx int = -1

	// Request list for selected collection
	requestList := widget.NewList(
		func() int {
			if workspaceSelect.Selected == "" || selectedCollectionIdx < 0 {
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
			return widget.NewLabel("")
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			for _, ws := range workspaces {
				if ws.Name == workspaceSelect.Selected {
					if selectedCollectionIdx < len(ws.Collections) {
						requests := ws.Collections[selectedCollectionIdx].Requests
						if i < len(requests) {
							label := o.(*widget.Label)
							label.SetText(requests[i].Name)
						}
					}
				}
			}
		},
	)
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

	collectionList.OnSelected = func(id int) {
		selectedCollectionIdx = id
		// Refresh request list when collection changes
		requestList.Refresh()
	}

	// Flows canvas placeholder
	flowsLabel := widget.NewLabel("Flows canvas: Drag and chain API calls here (future)")

	// JSONata search UI
	jsonataEntry := widget.NewEntry()
	jsonataEntry.SetPlaceHolder("Enter JSONata expression (e.g. $.foo.bar)")
	jsonataBtn := widget.NewButton("Apply JSONata", func() {
		expr := jsonataEntry.Text
		var jsonData interface{}
		err := json.Unmarshal([]byte(jsonResponse.Text), &jsonData)
		if err != nil {
			dialog.ShowError(fmt.Errorf("Invalid JSON: %v", err), w)
			return
		}
		res, err := jsonpath.Get(expr, jsonData)
		if err != nil {
			dialog.ShowError(fmt.Errorf("JSONata error: %v", err), w)
			return
		}
		resStr, _ := json.MarshalIndent(res, "", "  ")
		jsonResponse.SetText(string(resStr))
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
			return
		}
		defer resp.Body.Close()
		respBody, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			jsonResponse.SetText(fmt.Sprintf("Read error: %v", err))
			// statusLabel.SetText("")
			headersBox.SetText("")
			responseMeta.SetText("")
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
		if workspaceSelect.Selected == "" {
			dialog.ShowInformation("No Workspace", "Please select a workspace.", w)
			return
		}
		if collectionList.Length() == 0 {
			dialog.ShowInformation("No Collection", "Please add a collection in this workspace.", w)
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
		if selectedCollectionIdx < 0 {
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

	// Add workspace and collection creation/removal
	addWorkspaceBtn := widget.NewButton("+ Workspace", func() {
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
				workspaceNames = append(workspaceNames, entry.Text)
				workspaceSelect.Options = workspaceNames
				workspaceSelect.SetSelected(entry.Text)
			}
		}, w)
		form.Show()
	})

	addCollectionBtn := widget.NewButton("+ Collection", func() {
		if workspaceSelect.Selected == "" {
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
						collectionList.Refresh()
					}
				}
			}
		}, w)
		form.Show()
	})

	// Import/Export Postman Collection
	importBtn := widget.NewButton("Import Postman JSON", func() {
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
					collectionList.Refresh()
				}
			}
		}, w)
	})

	exportBtn := widget.NewButton("Export Collection as JSON", func() {
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
	})

	// --- UI Layout Improvements ---
	// Sidebar: vertical, with clear sectioning and spacing
	sidebar := container.NewVBox(
		container.NewHBox(addWorkspaceBtn, layout.NewSpacer(), addCollectionBtn),
		widget.NewLabelWithStyle("Workspaces", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		workspaceSelect,
		widget.NewLabelWithStyle("Collections", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewVBox(collectionList),
		widget.NewLabelWithStyle("Requests", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewVBox(requestList),
		container.NewHBox(importBtn, exportBtn),
		widget.NewSeparator(),
		flowsLabel,
	)

	// Request Row: method, URL, Send button (URL entry with larger width and resizable)
	urlEntry.MultiLine = false
	urlEntry.Wrapping = fyne.TextWrapOff
	urlSplit := container.NewHSplit(methodSelect, urlEntry)
	urlSplit.Offset = 0.15 // Start with method select smaller
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
		jsonataSplit,
	))
	responseTabs = container.NewAppTabs(
		container.NewTabItem("JSON", container.NewVBox(responseControls, jsonResponseScroller)),
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
	w.SetContent(container.NewHSplit(
		container.NewVBox(sidebar),
		container.NewVScroll(rightPane),
	))
	w.Resize(fyne.NewSize(2000, 1200))
	urlEntry.Resize(fyne.NewSize(900, urlEntry.MinSize().Height)) // Set width after window is created
	w.ShowAndRun()
}
