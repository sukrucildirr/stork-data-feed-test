package main

import (
        "encoding/json"
        "fmt"
        "html/template"
        "net/http"
        "sync"
        "time"
)

var (
        priceFeedsMutex sync.RWMutex
        priceFeedsCache map[string]*PriceFeed
)

func startWebServer(priceFeeds map[string]*PriceFeed) {
        priceFeedsCache = priceFeeds
        
        // Serve static files
        http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
        
        // API endpoint for price data
        http.HandleFunc("/api/prices", func(w http.ResponseWriter, r *http.Request) {
                priceFeedsMutex.RLock()
                defer priceFeedsMutex.RUnlock()
                
                w.Header().Set("Content-Type", "application/json")
                json.NewEncoder(w).Encode(priceFeedsCache)
        })
        
        // Web UI
        http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
                tmpl, err := template.ParseFiles("templates/index.html")
                if err != nil {
                        http.Error(w, "Failed to load template", http.StatusInternalServerError)
                        return
                }
                
                priceFeedsMutex.RLock()
                defer priceFeedsMutex.RUnlock()
                
                data := struct {
                        LastUpdate time.Time
                        PriceFeeds map[string]*PriceFeed
                }{
                        LastUpdate: time.Now(),
                        PriceFeeds: priceFeedsCache,
                }
                
                tmpl.Execute(w, data)
        })
        
        fmt.Println("Web server starting on http://localhost:8080")
        http.ListenAndServe(":8080", nil)
}