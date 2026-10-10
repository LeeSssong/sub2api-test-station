import {it,expect} from 'vitest';
import {getThumbUrl} from '../../lib/image-url';
it('keeps protected local image URLs direct so browser session cookies reach the media route',()=>{
 const url='/image-workstation/api/workstation/media/site_1_12345678-1234-1234-1234-123456789abc/aabb.png';
 expect(getThumbUrl(url,640)).toBe(url);
});
