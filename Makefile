VERSION_FILE := version.txt
CURRENT_VERSION := $(shell cat $(VERSION_FILE) 2>/dev/null || echo "v0.0.0")

.PHONY: version
version:
	@echo "Current version: $(CURRENT_VERSION)"

.PHONY: release-major
release-major:
	@echo "Current version: $(CURRENT_VERSION)"
	$(eval NEW_VERSION := $(shell echo $(CURRENT_VERSION) | awk -F. '{ printf "v%d.0.0", $$1+1 }'))
	@echo "New version: $(NEW_VERSION)"
	@echo $(NEW_VERSION) > $(VERSION_FILE)
	git add $(VERSION_FILE)
	git commit -m "Bump version to $(NEW_VERSION)"
	git tag -a $(NEW_VERSION) -m "Release $(NEW_VERSION)"
	@echo "Run 'git push && git push --tags' to publish"

.PHONY: release-minor
release-minor:
	@echo "Current version: $(CURRENT_VERSION)"
	$(eval NEW_VERSION := $(shell echo $(CURRENT_VERSION) | awk -F. '{ printf "v%d.%d.0", $$1, $$2+1 }'))
	@echo "New version: $(NEW_VERSION)"
	@echo $(NEW_VERSION) > $(VERSION_FILE)
	git add $(VERSION_FILE)
	git commit -m "Bump version to $(NEW_VERSION)"
	git tag -a $(NEW_VERSION) -m "Release $(NEW_VERSION)"
	@echo "Run 'git push && git push --tags' to publish"

.PHONY: release-patch
release-patch:
	@echo "Current version: $(CURRENT_VERSION)"
	$(eval NEW_VERSION := $(shell echo $(CURRENT_VERSION) | awk -F. '{ printf "v%d.%d.%d", $$1, $$2, $$3+1 }'))
	@echo "New version: $(NEW_VERSION)"
	@echo $(NEW_VERSION) > $(VERSION_FILE)
	git add $(VERSION_FILE)
	git commit -m "Bump version to $(NEW_VERSION)"
	git tag -a $(NEW_VERSION) -m "Release $(NEW_VERSION)"
	@echo "Run 'git push && git push --tags' to publish"