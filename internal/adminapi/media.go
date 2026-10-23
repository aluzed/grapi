package adminapi

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/aluzed/grapi/internal/auth/permissions"
	"github.com/aluzed/grapi/internal/db/query"
	"github.com/aluzed/grapi/internal/httpapi"
)

const (
	fileUID   = "plugin::upload.file"
	folderUID = "plugin::upload.folder"

	uploadRead   = "plugin::upload.read"
	uploadCreate = "plugin::upload.assets.create"
	uploadUpdate = "plugin::upload.assets.update"
)

// MediaService is the upload pipeline of the content API.
type MediaService interface {
	AdminUpload(w http.ResponseWriter, r *http.Request)
	AdminDeleteFile(ctx context.Context, id any) (query.Row, error)
	SignFile(ctx context.Context, row query.Row)
}

func (s *Server) mediaRoutes() {
	if _, ok := s.set.ContentType(fileUID); !ok || s.cfg.Media == nil {
		return
	}
	s.allowed("GET /upload/files", uploadRead, s.listFiles)
	s.allowed("GET /upload/files/{id}", uploadRead, s.getFile)
	s.allowed("PUT /upload/files/{id}", uploadUpdate, s.updateFile)
	s.allowed("DELETE /upload/files/{id}", uploadUpdate, s.deleteFile)
	s.allowed("POST /upload", uploadCreate, s.uploadFiles)
	s.allowed("GET /upload/folders", uploadRead, s.listFolders)
	s.allowed("POST /upload/folders", uploadCreate, s.createFolder)
}

func fileID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return 0, errNotFound
	}
	return id, nil
}

// folderWhere is the folder filter of a ?folder= value: an id, or the root
// when empty.
func folderWhere(v string) (map[string]any, error) {
	if v == "" || v == "root" {
		return map[string]any{"folder": map[string]any{"id": map[string]any{"$null": true}}}, nil
	}
	id, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return nil, validation([]FieldError{{Path: []string{"folder"}, Message: "folder must be an id"}})
	}
	return map[string]any{"folder": map[string]any{"id": id}}, nil
}

var fileSorts = map[string]bool{"name": true, "createdAt": true, "updatedAt": true, "size": true}

func (s *Server) listFiles(w http.ResponseWriter, r *http.Request, _ *User, a *permissions.Ability) error {
	q := r.URL.Query()
	page, size := pageParams(r)
	var parts []any
	if q.Get("all") != "true" {
		f, err := folderWhere(q.Get("folder"))
		if err != nil {
			return err
		}
		parts = append(parts, f)
	}
	if v := strings.TrimSpace(q.Get("_q")); v != "" {
		parts = append(parts, map[string]any{"$or": []any{
			map[string]any{"name": map[string]any{"$containsi": v}},
			map[string]any{"alternativeText": map[string]any{"$containsi": v}},
			map[string]any{"caption": map[string]any{"$containsi": v}},
		}})
	}
	switch q.Get("type") {
	case "image", "video", "audio":
		parts = append(parts, map[string]any{"mime": map[string]any{"$startsWith": q.Get("type") + "/"}})
	case "file":
		parts = append(parts, map[string]any{"$and": []any{
			map[string]any{"mime": map[string]any{"$notContainsi": "image/"}},
			map[string]any{"mime": map[string]any{"$notContainsi": "video/"}},
			map[string]any{"mime": map[string]any{"$notContainsi": "audio/"}},
		}})
	}
	if f, _ := a.Filter(uploadRead, ""); f != nil {
		parts = append(parts, f)
	}
	where := and(parts...)
	sortBy, dir, _ := strings.Cut(q.Get("sort"), ":")
	if !fileSorts[sortBy] {
		sortBy = "createdAt"
		dir = "desc"
	}
	if dir != "asc" {
		dir = "desc"
	}
	ctx := r.Context()
	total, err := s.e.Count(ctx, fileUID, query.Params{Where: where})
	if err != nil {
		return err
	}
	offset := (page - 1) * size
	rows, err := s.e.FindMany(ctx, fileUID, query.Params{Where: where, Limit: &size, Offset: &offset, OrderBy: []any{map[string]any{sortBy: dir}, map[string]any{"id": "desc"}}})
	if err != nil {
		return err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		s.cfg.Media.SignFile(ctx, row)
		out = append(out, toMap(row))
	}
	pages := int((total + int64(size) - 1) / int64(size))
	writeJSON(w, http.StatusOK, map[string]any{"data": out, "meta": map[string]any{"pagination": Pagination{Page: page, PageSize: size, PageCount: pages, Total: total}}})
	return nil
}

func (s *Server) fileRow(ctx context.Context, id int64) (query.Row, error) {
	row, err := s.e.FindOne(ctx, fileUID, query.Params{Where: map[string]any{"id": id}, Populate: map[string]any{"folder": true}})
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, &Error{Status: http.StatusNotFound, Name: "NotFoundError", Message: "File not found"}
	}
	return row, nil
}

func (s *Server) getFile(w http.ResponseWriter, r *http.Request, _ *User, _ *permissions.Ability) error {
	id, err := fileID(r)
	if err != nil {
		return err
	}
	row, err := s.fileRow(r.Context(), id)
	if err != nil {
		return err
	}
	s.cfg.Media.SignFile(r.Context(), row)
	return data(w, toMap(row))
}

// writableFile checks the row filter of assets.update (is-creator).
func (s *Server) writableFile(ctx context.Context, a *permissions.Ability, id int64) error {
	f, _ := a.Filter(uploadUpdate, "")
	n, err := s.e.Count(ctx, fileUID, query.Params{Where: and(map[string]any{"id": id}, f)})
	if err != nil {
		return err
	}
	if n == 0 {
		return &Error{Status: http.StatusNotFound, Name: "NotFoundError", Message: "File not found"}
	}
	return nil
}

func (s *Server) updateFile(w http.ResponseWriter, r *http.Request, u *User, a *permissions.Ability) (err error) {
	defer func() {
		if err == nil {
			s.audit(r, u, "media.update", map[string]any{"id": r.PathValue("id")})
		}
	}()
	id, err := fileID(r)
	if err != nil {
		return err
	}
	ctx := r.Context()
	if err := s.writableFile(ctx, a, id); err != nil {
		return err
	}
	var b struct {
		Name            *string `json:"name"`
		AlternativeText *string `json:"alternativeText"`
		Caption         *string `json:"caption"`
		Folder          *int64  `json:"folder"`
		RootFolder      bool    `json:"rootFolder"`
	}
	if err := decode(r, &b); err != nil {
		return err
	}
	set := query.Data{"updatedBy": u.ID}
	if b.Name != nil {
		if strings.TrimSpace(*b.Name) == "" {
			return validation([]FieldError{{Path: []string{"name"}, Message: "name is a required field"}})
		}
		set["name"] = *b.Name
	}
	if b.AlternativeText != nil {
		set["alternativeText"] = nullable(*b.AlternativeText)
	}
	if b.Caption != nil {
		set["caption"] = nullable(*b.Caption)
	}
	switch {
	case b.RootFolder:
		set["folder"], set["folderPath"] = nil, "/"
	case b.Folder != nil:
		folder, err := s.e.FindOne(ctx, folderUID, query.Params{Where: map[string]any{"id": *b.Folder}})
		if err != nil {
			return err
		}
		if folder == nil {
			return validation([]FieldError{{Path: []string{"folder"}, Message: "Folder does not exist"}})
		}
		set["folder"], set["folderPath"] = *b.Folder, get(folder, "path")
	}
	if _, err := s.e.Update(ctx, fileUID, map[string]any{"id": id}, set, query.Params{}); err != nil {
		return err
	}
	row, err := s.fileRow(ctx, id)
	if err != nil {
		return err
	}
	s.cfg.Media.SignFile(ctx, row)
	return data(w, toMap(row))
}

func (s *Server) deleteFile(w http.ResponseWriter, r *http.Request, u *User, a *permissions.Ability) error {
	id, err := fileID(r)
	if err != nil {
		return err
	}
	if err := s.writableFile(r.Context(), a, id); err != nil {
		return err
	}
	row, err := s.cfg.Media.AdminDeleteFile(r.Context(), id)
	if err != nil {
		return err
	}
	if row == nil {
		return errNotFound
	}
	s.audit(r, u, "media.delete", map[string]any{"id": id, "name": getString(row, "name")})
	return data(w, toMap(row))
}

// uploadFiles runs the upload pipeline into ?folder=; with ?id= it updates
// or replaces that file.
func (s *Server) uploadFiles(w http.ResponseWriter, r *http.Request, u *User, a *permissions.Ability) error {
	ctx := r.Context()
	if v := r.URL.Query().Get("id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return errNotFound
		}
		if !a.Can(uploadUpdate, "", "") {
			return errForbidden
		}
		if err := s.writableFile(ctx, a, id); err != nil {
			return err
		}
	}
	var folder query.Row
	if v := r.URL.Query().Get("folder"); v != "" && v != "root" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return validation([]FieldError{{Path: []string{"folder"}, Message: "folder must be an id"}})
		}
		if folder, err = s.e.FindOne(ctx, folderUID, query.Params{Where: map[string]any{"id": id}}); err != nil {
			return err
		}
		if folder == nil {
			return validation([]FieldError{{Path: []string{"folder"}, Message: "Folder does not exist"}})
		}
	}
	event := "media.create"
	if r.URL.Query().Get("id") != "" {
		event = "media.update"
	}
	s.audit(r, u, event, map[string]any{"folder": r.URL.Query().Get("folder"), "id": r.URL.Query().Get("id")})
	s.cfg.Media.AdminUpload(w, r.WithContext(httpapi.WithUploader(ctx, u.ID, folder)))
	return nil
}

func (s *Server) listFolders(w http.ResponseWriter, r *http.Request, _ *User, _ *permissions.Ability) error {
	ctx := r.Context()
	var where any = map[string]any{"parent": map[string]any{"id": map[string]any{"$null": true}}}
	if r.URL.Query().Get("all") == "true" {
		where = nil
	} else if v := r.URL.Query().Get("parent"); v != "" && v != "root" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return validation([]FieldError{{Path: []string{"parent"}, Message: "parent must be an id"}})
		}
		where = map[string]any{"parent": map[string]any{"id": id}}
	}
	order := []any{map[string]any{"name": "asc"}}
	if where == nil {
		order = []any{map[string]any{"path": "asc"}}
	}
	rows, err := s.e.FindMany(ctx, folderUID, query.Params{Where: where, OrderBy: order})
	if err != nil {
		return err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		m := toMap(row)
		id := get(row, "id")
		files, err := s.e.Count(ctx, fileUID, query.Params{Where: map[string]any{"folder": map[string]any{"id": id}}})
		if err != nil {
			return err
		}
		children, err := s.e.Count(ctx, folderUID, query.Params{Where: map[string]any{"parent": map[string]any{"id": id}}})
		if err != nil {
			return err
		}
		m["filesCount"], m["childrenCount"] = files, children
		out = append(out, m)
	}
	return data(w, out)
}

// createFolder numbers the folder after the highest pathId, as Strapi's
// folder service does: its path is the parent's path plus that number.
func (s *Server) createFolder(w http.ResponseWriter, r *http.Request, u *User, _ *permissions.Ability) error {
	var b struct {
		Name   string `json:"name"`
		Parent *int64 `json:"parent"`
	}
	if err := decode(r, &b); err != nil {
		return err
	}
	name := strings.TrimSpace(b.Name)
	if name == "" {
		return validation([]FieldError{{Path: []string{"name"}, Message: "name is a required field"}})
	}
	ctx := r.Context()
	parentPath := ""
	var parent any
	siblings := map[string]any{"parent": map[string]any{"id": map[string]any{"$null": true}}}
	if b.Parent != nil {
		p, err := s.e.FindOne(ctx, folderUID, query.Params{Where: map[string]any{"id": *b.Parent}})
		if err != nil {
			return err
		}
		if p == nil {
			return validation([]FieldError{{Path: []string{"parent"}, Message: "Parent folder does not exist"}})
		}
		parentPath, parent = getString(p, "path"), *b.Parent
		siblings = map[string]any{"parent": map[string]any{"id": *b.Parent}}
	}
	if n, err := s.e.Count(ctx, folderUID, query.Params{Where: and(siblings, map[string]any{"name": name})}); err != nil {
		return err
	} else if n > 0 {
		return validation([]FieldError{{Path: []string{"name"}, Message: "A folder with this name already exists"}})
	}
	all, err := s.e.FindMany(ctx, folderUID, query.Params{Select: []string{"pathId"}})
	if err != nil {
		return err
	}
	maxID := int64(0)
	for _, f := range all {
		if v := toInt64(get(f, "pathId")); v > maxID {
			maxID = v
		}
	}
	pathID := maxID + 1
	row, err := s.e.Create(ctx, folderUID, query.Data{
		"name": name, "pathId": pathID, "path": parentPath + "/" + strconv.FormatInt(pathID, 10),
		"parent": parent, "createdBy": u.ID, "updatedBy": u.ID,
	}, query.Params{})
	if err != nil {
		return err
	}
	s.audit(r, u, "media-folder.create", map[string]any{"id": get(row, "id"), "name": name})
	writeJSON(w, http.StatusCreated, map[string]any{"data": toMap(row)})
	return nil
}

func (s *Server) mediaBulkRoutes() {
	if _, ok := s.set.ContentType(fileUID); !ok || s.cfg.Media == nil {
		return
	}
	s.allowed("PUT /upload/folders/{id}", uploadUpdate, s.renameFolder)
	s.allowed("DELETE /upload/folders/{id}", uploadUpdate, s.deleteFolder)
	s.allowed("POST /upload/actions/bulk-delete", uploadUpdate, s.bulkDeleteMedia)
	s.allowed("POST /upload/actions/bulk-move", uploadUpdate, s.bulkMoveFiles)
}

func (s *Server) folderByPath(r *http.Request) (query.Row, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return nil, errNotFound
	}
	row, err := s.e.FindOne(r.Context(), folderUID, query.Params{Where: map[string]any{"id": id}})
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, &Error{Status: http.StatusNotFound, Name: "NotFoundError", Message: "Folder not found"}
	}
	return row, nil
}

func (s *Server) renameFolder(w http.ResponseWriter, r *http.Request, u *User, _ *permissions.Ability) error {
	row, err := s.folderByPath(r)
	if err != nil {
		return err
	}
	var b struct {
		Name string `json:"name"`
	}
	if err := decode(r, &b); err != nil {
		return err
	}
	name := strings.TrimSpace(b.Name)
	if name == "" {
		return validation([]FieldError{{Path: []string{"name"}, Message: "name is a required field"}})
	}
	ctx := r.Context()
	parent, err := s.e.FindOne(ctx, folderUID, query.Params{Where: map[string]any{"id": get(row, "id")}, Populate: map[string]any{"parent": true}})
	if err != nil {
		return err
	}
	siblings := map[string]any{"parent": map[string]any{"id": map[string]any{"$null": true}}}
	if p, ok := get(parent, "parent").(query.Row); ok && p != nil {
		siblings = map[string]any{"parent": map[string]any{"id": get(p, "id")}}
	}
	if n, err := s.e.Count(ctx, folderUID, query.Params{Where: and(siblings, map[string]any{"name": name}, map[string]any{"id": map[string]any{"$ne": get(row, "id")}})}); err != nil {
		return err
	} else if n > 0 {
		return validation([]FieldError{{Path: []string{"name"}, Message: "A folder with this name already exists"}})
	}
	out, err := s.e.Update(ctx, folderUID, map[string]any{"id": get(row, "id")}, query.Data{"name": name, "updatedBy": u.ID}, query.Params{})
	if err != nil {
		return err
	}
	s.audit(r, u, "media-folder.update", map[string]any{"id": get(row, "id"), "name": name})
	return data(w, toMap(out))
}

// removeFolders deletes folders with everything under them: subfolders by
// path, files by folderPath, as Strapi's folder deletion does.
func (s *Server) removeFolders(ctx context.Context, ids []int64) (files, folders int, err error) {
	for _, id := range ids {
		row, err := s.e.FindOne(ctx, folderUID, query.Params{Where: map[string]any{"id": id}})
		if err != nil {
			return files, folders, err
		}
		if row == nil {
			continue
		}
		path := getString(row, "path")
		under := []any{map[string]any{"path": path}, map[string]any{"path": map[string]any{"$startsWith": path + "/"}}}
		fileRows, err := s.e.FindMany(ctx, fileUID, query.Params{Where: map[string]any{"$or": []any{
			map[string]any{"folderPath": path}, map[string]any{"folderPath": map[string]any{"$startsWith": path + "/"}},
		}}, Select: []string{"id"}})
		if err != nil {
			return files, folders, err
		}
		for _, f := range fileRows {
			if _, err := s.cfg.Media.AdminDeleteFile(ctx, get(f, "id")); err != nil {
				return files, folders, err
			}
			files++
		}
		n, err := s.e.DeleteMany(ctx, folderUID, query.Params{Where: map[string]any{"$or": under}})
		if err != nil {
			return files, folders, err
		}
		folders += int(n)
	}
	return files, folders, nil
}

func (s *Server) deleteFolder(w http.ResponseWriter, r *http.Request, u *User, _ *permissions.Ability) error {
	row, err := s.folderByPath(r)
	if err != nil {
		return err
	}
	files, folders, err := s.removeFolders(r.Context(), []int64{toInt64(get(row, "id"))})
	if err != nil {
		return err
	}
	s.audit(r, u, "media-folder.delete", map[string]any{"id": get(row, "id"), "name": getString(row, "name"), "files": files, "folders": folders})
	return data(w, map[string]any{"files": files, "folders": folders})
}

type mediaSelection struct {
	FileIDs   []int64 `json:"fileIds"`
	FolderIDs []int64 `json:"folderIds"`
	// Destination is the target folder of a move, nil for the root.
	Destination *int64 `json:"destination"`
}

func (s *Server) bulkDeleteMedia(w http.ResponseWriter, r *http.Request, u *User, a *permissions.Ability) error {
	var b mediaSelection
	if err := decode(r, &b); err != nil {
		return err
	}
	ctx := r.Context()
	deleted, skipped := 0, 0
	for _, id := range b.FileIDs {
		if err := s.writableFile(ctx, a, id); err != nil {
			skipped++
			continue
		}
		row, err := s.cfg.Media.AdminDeleteFile(ctx, id)
		if err != nil {
			return err
		}
		if row != nil {
			deleted++
		}
	}
	files, folders, err := s.removeFolders(ctx, b.FolderIDs)
	if err != nil {
		return err
	}
	s.audit(r, u, "media.delete", map[string]any{"fileIds": b.FileIDs, "folderIds": b.FolderIDs})
	return data(w, map[string]any{"files": deleted + files, "folders": folders, "skipped": skipped})
}

func (s *Server) bulkMoveFiles(w http.ResponseWriter, r *http.Request, u *User, a *permissions.Ability) error {
	var b mediaSelection
	if err := decode(r, &b); err != nil {
		return err
	}
	ctx := r.Context()
	set := query.Data{"folder": nil, "folderPath": "/", "updatedBy": u.ID}
	if b.Destination != nil {
		folder, err := s.e.FindOne(ctx, folderUID, query.Params{Where: map[string]any{"id": *b.Destination}})
		if err != nil {
			return err
		}
		if folder == nil {
			return validation([]FieldError{{Path: []string{"destination"}, Message: "Folder does not exist"}})
		}
		set["folder"], set["folderPath"] = *b.Destination, get(folder, "path")
	}
	moved := 0
	for _, id := range b.FileIDs {
		if err := s.writableFile(ctx, a, id); err != nil {
			continue
		}
		if _, err := s.e.Update(ctx, fileUID, map[string]any{"id": id}, set, query.Params{}); err != nil {
			return err
		}
		moved++
	}
	s.audit(r, u, "media.update", map[string]any{"fileIds": b.FileIDs, "destination": b.Destination})
	return data(w, map[string]any{"files": moved})
}
