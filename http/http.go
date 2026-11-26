package fbhttp

import (
	"io/fs"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/spf13/afero"

	"github.com/filebrowser/filebrowser/v2/files"
	"github.com/filebrowser/filebrowser/v2/settings"
	"github.com/filebrowser/filebrowser/v2/storage"
)

type modifyRequest struct {
	What            string   `json:"what"`             // Answer to: what data type?
	Which           []string `json:"which"`            // Answer to: which fields?
	CurrentPassword string   `json:"current_password"` // Answer to: user logged password
}

func NewHandler(
	imgSvc ImgService,
	fileCache FileCache,
	uploadCache UploadCache,
	store *storage.Storage,
	server *settings.Server,
	assetsFs fs.FS,
) (http.Handler, error) {
	server.Clean()
	server.CaseInsensitiveFs = files.CaseInsensitive(afero.NewOsFs(), server.Root)

	r := mux.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Security-Policy", `default-src 'self'; style-src 'unsafe-inline';`)
			next.ServeHTTP(w, r)
		})
	})
	index, static := getStaticHandlers(store, server, assetsFs)

	monkey := func(fn handleFunc, prefix string) http.Handler {
		return handle(fn, prefix, store, server)
	}

	r.HandleFunc("/health", healthHandler)
	r.PathPrefix("/static").Handler(static)
	r.NotFoundHandler = index

	api := r.PathPrefix("/api").Subrouter()

	tokenExpirationTime := server.GetTokenExpirationTime(DefaultTokenExpirationTime)
	api.Handle("/login", monkey(loginHandler(tokenExpirationTime), ""))
	api.Handle("/signup", monkey(signupHandler, ""))
	api.Handle("/renew", monkey(renewHandler(tokenExpirationTime), ""))
	api.Handle("/auth/magic-link", monkey(magicLinkHandler, "")).Methods("POST")
	api.Handle("/admin/backups/destinations", monkey(backupDestinationsHandler, "")).Methods("GET", "POST")
	api.Handle("/admin/backups/source-folders", monkey(backupSourceFoldersHandler, "")).Methods("GET")
	api.Handle("/admin/backups/jobs", monkey(backupJobsHandler, "")).Methods("GET", "POST", "DELETE")
	api.Handle("/admin/backups/restores", monkey(backupRestoresHandler, "")).Methods("GET", "POST")
	api.Handle("/admin/backups/catalog", monkey(backupCatalogHandler, "")).Methods("GET")
	api.Handle("/admin/backups/schedules", monkey(backupSchedulesHandler, "")).Methods("GET", "POST")
	api.Handle("/desktop/status", monkey(desktopDownloadStatusHandler, "")).Methods("GET")
	api.Handle("/sync/device/capabilities", monkey(driveSyncCapabilitiesHandler, "")).Methods("GET")
	api.Handle("/sync/device/upload", monkey(driveSyncUploadHandler, "")).Methods("POST")
	api.Handle("/sync/device/mkdir", monkey(driveSyncMkdirHandler, "")).Methods("POST")
	api.Handle("/sync/device/download", monkey(driveSyncDownloadHandler, "")).Methods("GET")
	api.Handle("/desktop/download", monkey(desktopDownloadHandler, "")).Methods("GET")
	api.Handle("/sync/pair/start", monkey(desktopPairStartHandler, "")).Methods("POST")
	api.Handle("/sync/pair/request", monkey(desktopPairInfoHandler, "")).Methods("GET")
	api.Handle("/sync/pair/approve", monkey(desktopPairApproveHandler, "")).Methods("POST")
	api.Handle("/sync/pair/claim", monkey(desktopPairClaimHandler, "")).Methods("POST")

	users := api.PathPrefix("/users").Subrouter()
	users.Handle("", monkey(usersGetHandler, "")).Methods("GET")
	users.Handle("", monkey(userPostHandler, "")).Methods("POST")
	users.Handle("/{id:[0-9]+}", monkey(userPutHandler, "")).Methods("PUT")
	users.Handle("/{id:[0-9]+}", monkey(userGetHandler, "")).Methods("GET")
	users.Handle("/{id:[0-9]+}", monkey(userDeleteHandler, "")).Methods("DELETE")

	api.PathPrefix("/resources/recursive").Handler(monkey(resourceGetRecursiveHandler, "/api/resources/recursive")).Methods("GET")
	api.PathPrefix("/resources").Handler(monkey(resourceGetHandler, "/api/resources")).Methods("GET")
	api.PathPrefix("/resources").Handler(monkey(resourceDeleteHandler(fileCache), "/api/resources")).Methods("DELETE")
	api.PathPrefix("/resources").Handler(monkey(resourcePostHandler(fileCache), "/api/resources")).Methods("POST")
	api.PathPrefix("/resources").Handler(monkey(resourcePutHandler, "/api/resources")).Methods("PUT")
	api.PathPrefix("/resources").Handler(monkey(resourcePatchHandler(fileCache), "/api/resources")).Methods("PATCH")

	api.PathPrefix("/tus").Handler(monkey(tusPostHandler(uploadCache), "/api/tus")).Methods("POST")
	api.PathPrefix("/tus").Handler(monkey(tusHeadHandler(uploadCache), "/api/tus")).Methods("HEAD", "GET")
	api.PathPrefix("/tus").Handler(monkey(tusPatchHandler(uploadCache), "/api/tus")).Methods("PATCH")
	api.PathPrefix("/tus").Handler(monkey(tusDeleteHandler(uploadCache), "/api/tus")).Methods("DELETE")

	api.PathPrefix("/usage").Handler(monkey(diskUsage, "/api/usage")).Methods("GET")

	api.Handle("/shares", monkey(shareListHandler, "")).Methods("GET")
	api.Handle("/share-slug/{slug}", monkey(shareSlugHandler, "")).Methods("GET")

	api.Handle("/starred", monkey(starredGetHandler, "")).Methods("GET")
	api.Handle("/starred", monkey(starredPutHandler, "")).Methods("PUT")
	api.Handle("/recent", monkey(recentHandler, "")).Methods("GET")
	api.Handle("/sync/inventory", monkey(driveSyncInventoryHandler, "")).Methods("GET")
	api.Handle("/sync/devices", monkey(driveSyncDevicesHandler, "")).Methods("GET", "POST", "DELETE")
	api.Handle("/sync/device/inventory", monkey(driveSyncDeviceInventoryHandler, "")).Methods("GET")
	api.PathPrefix("/share").Handler(monkey(shareGetsHandler, "/api/share")).Methods("GET")
	api.PathPrefix("/share").Handler(monkey(sharePostHandler, "/api/share")).Methods("POST")
	api.PathPrefix("/share").Handler(monkey(shareDeleteHandler, "/api/share")).Methods("DELETE")

	api.Handle("/settings", monkey(settingsGetHandler, "")).Methods("GET")
	api.Handle("/settings", monkey(settingsPutHandler, "")).Methods("PUT")

	api.PathPrefix("/raw").Handler(monkey(rawHandler, "/api/raw")).Methods("GET")
	api.PathPrefix("/preview/{size}/{path:.*}").
		Handler(monkey(previewHandler(imgSvc, fileCache, server.EnableThumbnails, server.ResizePreview), "/api/preview")).Methods("GET")
	api.PathPrefix("/command").Handler(monkey(commandsHandler, "/api/command")).Methods("GET")
	api.PathPrefix("/search").Handler(monkey(searchHandler, "/api/search")).Methods("GET")
	api.PathPrefix("/subtitle").Handler(monkey(subtitleHandler, "/api/subtitle")).Methods("GET")

	public := api.PathPrefix("/public").Subrouter()
	public.PathPrefix("/dl").Handler(monkey(publicDlHandler, "/api/public/dl/")).Methods("GET")
	public.PathPrefix("/share").Handler(monkey(publicShareHandler, "/api/public/share/")).Methods("GET")

	return stripPrefix(server.BaseURL, r), nil
}
