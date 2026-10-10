import {defineConfig} from 'vitest/config';
export default defineConfig({test:{environment:'node',include:['src/tests/unit/site-*.test.ts'],pool:'forks',maxWorkers:1}});
