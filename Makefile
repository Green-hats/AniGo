.PHONY: ui build all dev clean test e2e

# 构建前端并拷贝到后端 embed 目录
ui:
	cd frontend && npm install && npm run build
	rm -rf backend/internal/httpapi/static
	mkdir -p backend/internal/httpapi/static
	cp -r frontend/dist/. backend/internal/httpapi/static/

# 构建后端（使用已嵌入的前端）
build:
	cd backend && go build -o bin/anigo ./cmd/anigo

# 前端 + 后端一次性构建
all: ui build

# 开发：后端 + 前端 Vite dev server（/api 代理）
dev:
	cd backend && go run ./cmd/anigo &
	cd frontend && npm run dev

# 运行后端
run:
	cd backend && go run ./cmd/anigo

clean:
	rm -rf frontend/node_modules frontend/dist backend/bin backend/internal/httpapi/static

test:
	cd backend && go vet ./... && go test ./...

# 端到端集成测试（需要真实外部服务：AI/115/BGM/TMDB/animes.garden）
e2e:
	bash scripts/e2e.sh