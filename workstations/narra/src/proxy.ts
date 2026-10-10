import { NextRequest, NextResponse } from 'next/server';
import { allowedWorkstationPath } from './lib/workstation/policy';
export function proxy(request: NextRequest) {
 if (!allowedWorkstationPath(request.nextUrl.pathname)) return new NextResponse('Not found', { status: 404 });
 if (!['GET','HEAD','OPTIONS'].includes(request.method)) {
  const origin = request.headers.get('origin');
  if (!origin || origin !== new URL(process.env.APP_URL!).origin) return new NextResponse('Forbidden', {status:403});
 }
 return NextResponse.next();
}
export const config = { matcher: ['/((?!_next/static|_next/image).*)'] };
