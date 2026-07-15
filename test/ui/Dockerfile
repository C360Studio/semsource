FROM mcr.microsoft.com/playwright:v1.61.1-noble@sha256:5b8f294aff9041b7191c34a4bab3ac270157a28774d4b0660e9743297b697e48

WORKDIR /work
COPY ui/package.json ui/package-lock.json ./
RUN npm ci --ignore-scripts
COPY test/ui/playwright.config.cjs ./playwright.config.cjs
COPY test/ui/ui-profile.spec.cjs ./ui-profile.spec.cjs

CMD ["npx", "playwright", "test", "--config", "playwright.config.cjs"]
