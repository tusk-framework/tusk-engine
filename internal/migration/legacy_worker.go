// Package migration inspects legacy projects and adds user-owned modern skeleton files.
package migration

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type State string

const (
	Legacy  State = "legacy"
	Modern  State = "modern"
	Missing State = "missing"
)

type Detection struct {
	State   State  `json:"state"`
	Message string `json:"message"`
	Action  string `json:"action,omitempty"`
}

// Detect inspects entrypoint paths. It never loads or executes PHP.
func Detect(root string) (Detection, error) {
	if root == "" {
		return Detection{}, fmt.Errorf("project root is required")
	}
	worker := filepath.Join(root, "worker.php")
	bootstrap := filepath.Join(root, "bootstrap", "app.php")
	workerInfo, workerErr := os.Lstat(worker)
	if workerErr != nil && !errors.Is(workerErr, os.ErrNotExist) {
		return Detection{}, fmt.Errorf("inspect %s: %w", worker, workerErr)
	}
	bootstrapInfo, bootstrapErr := os.Lstat(bootstrap)
	if bootstrapErr != nil && !errors.Is(bootstrapErr, os.ErrNotExist) {
		return Detection{}, fmt.Errorf("inspect %s: %w", bootstrap, bootstrapErr)
	}
	if workerErr == nil {
		if !workerInfo.Mode().IsRegular() {
			return Detection{}, fmt.Errorf("legacy worker path %s is not a regular file; inspect it manually", worker)
		}
		return Detection{Legacy, "Legacy root worker.php is incompatible with the generated RoadRunner worker path .tusk/runtime/worker.php; it will not be executed automatically.", "Run tusk migrate, review the created bootstrap and routes, then move worker.php aside before tusk start."}, nil
	}
	if bootstrapErr == nil {
		if !bootstrapInfo.Mode().IsRegular() {
			return Detection{}, fmt.Errorf("bootstrap path %s is not a regular file; inspect it manually", bootstrap)
		}
		return Detection{Modern, "Modern bootstrap/app.php is present.", "Run tusk start after confirming the application bootstrap and dependencies."}, nil
	}
	return Detection{Missing, "No root worker.php or bootstrap/app.php was found; there is no application entrypoint to migrate.", "Run tusk init for a new project or add bootstrap/app.php manually, then run tusk doctor."}, nil
}

type skeletonFile struct{ path, content string }

// These user-owned files follow the Framework generator's application builder layout.
// Routes and providers are intentionally empty because legacy application composition
// cannot be inferred safely from a worker script.
var skeleton = []skeletonFile{
	{"bootstrap/app.php", `<?php

use Tusk\Foundation\Application;

return Application::configure(dirname(__DIR__))
    ->withRouting(web: 'routes/web.php')
    ->withProviders(['bootstrap/providers.php'])
    ->create();
`},
	{"bootstrap/providers.php", `<?php

use Tusk\Core\Container\Container;

return static function (Container $container): void {
    // Register the services used by this application.
};
`},
	{"config/app.php", `<?php

return [
    'name' => 'Tusk',
];
`},
	{"routes/web.php", `<?php

use Tusk\Web\Router\Router;

return static function (Router $router): void {
    // Move your legacy routes here before starting the application.
};
`},
	{"public/index.php", `<?php

use Nyholm\Psr7\ServerRequest;

require __DIR__.'/../vendor/autoload.php';
$application = require __DIR__.'/../bootstrap/app.php';
$request = new ServerRequest(
    $_SERVER['REQUEST_METHOD'] ?? 'GET',
    $_SERVER['REQUEST_URI'] ?? '/',
    function_exists('getallheaders') ? getallheaders() : [],
    file_get_contents('php://input'),
    '1.1',
    $_SERVER,
);
$request = $request->withParsedBody($_POST)->withCookieParams($_COOKIE);
$response = $application->handle($request);
http_response_code($response->getStatusCode());
foreach ($response->getHeaders() as $name => $values) {
    foreach ($values as $value) {
        header($name.': '.$value, false);
    }
}
echo $response->getBody();
`},
}

// Migrate creates only absent skeleton files. Any existing target is a conflict,
// even if its contents resemble a generated template.
func Migrate(root string) ([]string, error) {
	detection, err := Detect(root)
	if err != nil {
		return nil, err
	}
	if detection.State == Missing {
		return nil, fmt.Errorf("%s %s", detection.Message, detection.Action)
	}
	if detection.State == Modern {
		return nil, fmt.Errorf("bootstrap/app.php already exists; review the modern project with tusk doctor; migration will not overwrite user-owned files")
	}
	var conflicts []string
	for _, file := range skeleton {
		path := filepath.Join(root, filepath.FromSlash(file.path))
		if _, err := os.Lstat(path); err == nil {
			conflicts = append(conflicts, file.path+": already exists; review and adapt this user-owned file manually")
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("inspect %s: %w", path, err)
		}
		parent := filepath.Dir(path)
		if info, err := os.Lstat(parent); err == nil {
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				conflicts = append(conflicts, filepath.ToSlash(strings.TrimPrefix(parent, root+string(os.PathSeparator)))+": not a safe directory; review it manually")
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("inspect %s: %w", parent, err)
		}
	}
	if len(conflicts) > 0 {
		return nil, fmt.Errorf("migration conflicts in %s:\n%s", root, strings.Join(conflicts, "\n"))
	}
	var created []string
	for _, file := range skeleton {
		path := filepath.Join(root, filepath.FromSlash(file.path))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return created, fmt.Errorf("create parent for %s: %w", file.path, err)
		}
		out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			return created, fmt.Errorf("create %s without overwrite: %w", file.path, err)
		}
		_, writeErr := out.WriteString(file.content)
		closeErr := out.Close()
		created = append(created, file.path)
		if writeErr != nil {
			return created, fmt.Errorf("write %s: %w", file.path, writeErr)
		}
		if closeErr != nil {
			return created, fmt.Errorf("close %s: %w", file.path, closeErr)
		}
	}
	return created, nil
}
