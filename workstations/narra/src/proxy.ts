import {NextRequest,NextResponse} from 'next/server';
import {allowedWorkstationPath} from './lib/workstation/policy';
import {sameOriginAllowed} from './lib/workstation/origin';
export function proxy(request:NextRequest){
 if(!allowedWorkstationPath(request.nextUrl.pathname))return new NextResponse('Not found',{status:404});
 if(!['GET','HEAD','OPTIONS'].includes(request.method)){
  const host=request.headers.get('x-forwarded-host')?.split(',')[0]?.trim()||request.headers.get('host');
  const protocol=request.headers.get('x-forwarded-proto')?.split(',')[0]?.trim()||request.nextUrl.protocol.replace(':','');
  if(!sameOriginAllowed(request.headers.get('origin'),host,protocol))return NextResponse.json({error:'入口来源校验失败，请从当前站点菜单重新打开工作站',code:'WORKSTATION_ORIGIN_MISMATCH'},{status:403});
 }
 return NextResponse.next();
}
export const config={matcher:['/((?!_next/static|_next/image).*)']};
