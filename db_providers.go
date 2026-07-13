package main

import (
	"database/sql"
	"path/filepath"
	"time"
)

// loadProvidersFromDB returns providers assembled from SQLite tables.
func loadProvidersFromDB(db *sql.DB) ([]Provider, error) {
	rows, err := db.Query(`SELECT id, name, base_url, api_key, color, use_proxy, format_standard FROM providers ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Provider
	index := map[string]int{}
	for rows.Next() {
		var p Provider
		var useProxy sql.NullInt64
		var fmtStd string
		if err := rows.Scan(&p.ID, &p.Name, &p.BaseURL, &p.APIKey, &p.Color, &useProxy, &fmtStd); err != nil {
			return nil, err
		}
		p.UseProxy = useProxyFromSQL(useProxy)
		p.FormatStandard = fmtStd
		if p.FormatStandard == "" {
			p.FormatStandard = "openai"
		}
		p.Models = []ProviderModel{}
		p.TokenPackages = []TokenPackage{}
		index[p.ID] = len(list)
		list = append(list, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	mrows, err := db.Query(`SELECT provider_id, model_id, name, enabled, is_default, owned_by FROM provider_models`)
	if err != nil {
		return nil, err
	}
	defer mrows.Close()
	for mrows.Next() {
		var pid, mid, name, owned string
		var en, def int
		if err := mrows.Scan(&pid, &mid, &name, &en, &def, &owned); err != nil {
			return nil, err
		}
		i, ok := index[pid]
		if !ok {
			continue
		}
		list[i].Models = append(list[i].Models, ProviderModel{
			ID: mid, Name: name, Enabled: intToBool(en), IsDefault: intToBool(def), OwnedBy: owned,
		})
	}

	prows, err := db.Query(`SELECT id, provider_id, name, total_tokens, used_offset, price, currency, start_at, expire_at, note, active FROM token_packages`)
	if err != nil {
		return nil, err
	}
	defer prows.Close()
	for prows.Next() {
		var pkg TokenPackage
		var pid string
		var active int
		if err := prows.Scan(&pkg.ID, &pid, &pkg.Name, &pkg.TotalTokens, &pkg.UsedOffset, &pkg.Price, &pkg.Currency, &pkg.StartAt, &pkg.ExpireAt, &pkg.Note, &active); err != nil {
			return nil, err
		}
		pkg.Active = intToBool(active)
		i, ok := index[pid]
		if !ok {
			continue
		}
		list[i].TokenPackages = append(list[i].TokenPackages, pkg)
	}
	return list, nil
}

// saveProvidersToDB replaces all providers and rebuilds model groups.
func saveProvidersToDB(db *sql.DB, list []Provider) error {
	return replaceProvidersInDB(db, list)
}

// loadProvidersFromDisk prefers SQLite; falls back to JSON if DB unavailable.
func loadProvidersFromDisk() ([]Provider, error) {
	db, err := openDB()
	if err != nil {
		// fallback legacy JSON only
		return loadProvidersJSONFile()
	}
	return loadProvidersFromDB(db)
}

// saveProvidersToDisk writes SQLite and keeps a JSON mirror for backup/compat.
func saveProvidersToDisk(list []Provider) error {
	db, err := openDB()
	if err != nil {
		if err := saveProvidersJSONFile(list); err != nil {
			return err
		}
		markProvidersInitializedJSON()
		return nil
	}
	if err := saveProvidersToDB(db, list); err != nil {
		return err
	}
	_ = metaSet(db, "providers_initialized", "1")
	// best-effort JSON mirror
	_ = saveProvidersJSONFile(list)
	return nil
}

func providersInitialized() bool {
	if db, err := openDB(); err == nil {
		return metaGet(db, "providers_initialized") == "1"
	}
	return fileExists(providersInitializedPath())
}

func markProvidersInitialized() {
	if db, err := openDB(); err == nil {
		_ = metaSet(db, "providers_initialized", "1")
		return
	}
	markProvidersInitializedJSON()
}

func providersInitializedPath() string {
	return filepath.Join(managerRoot(), "providers.initialized")
}

func markProvidersInitializedJSON() {
	_ = ensureManagerRoot()
	_ = writeFileAtomic(providersInitializedPath(), time.Now().Format(time.RFC3339)+"\n")
}

func saveProvidersJSONFile(list []Provider) error {
	if err := ensureManagerRoot(); err != nil {
		return err
	}
	f := providersFile{
		Version:   2,
		UpdatedAt: time.Now().Format(time.RFC3339),
		Providers: list,
	}
	return writeJSONFile(providersStorePath(), f)
}

func ensureManagerRoot() error {
	return mkManagerRoot()
}

func mkManagerRoot() error {
	return ensureDir(managerRoot())
}

func ensureDir(path string) error {
	return mkdirAll(path, 0o755)
}
