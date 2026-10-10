import {defineConfig} from 'vitest/config';
import path from 'node:path';
export default defineConfig({test:{environment:'node',include:['src/tests/unit/site-*.test.ts'],pool:'threads',maxWorkers:1},resolve:{alias:{'@':path.resolve(__dirname,'src')}}});
