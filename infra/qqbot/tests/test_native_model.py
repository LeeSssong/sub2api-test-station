import unittest,sys,tempfile
from pathlib import Path
sys.path.insert(0,str(Path(__file__).resolve().parents[3]))

class ModelTests(unittest.IsolatedAsyncioTestCase):
 async def test_tool_loop_records_only_final_turn(self):
  from infra.qqbot.service.model import Model
  from infra.qqbot.service.state import State
  with tempfile.TemporaryDirectory() as d:
   state=State(Path(d)/'db'); calls=[]
   async def transport(provider,messages,tools):
    calls.append(list(messages))
    if len(calls)==1:return {'role':'assistant','content':None,'tool_calls':[{'id':'t1','type':'function','function':{'name':'xingqiao_public_config','arguments':'{"query":"分组"}'}}]}
    return {'role':'assistant','content':'答复'}
   async def invoke(name,args):return '{"ok":true}'
   model=Model({'providers':[{'model':'fake'}],'persona':'小星','knowledge':'文档'},state,invoke,transport)
   self.assertEqual(await model.answer('s','问题'),'答复')
   self.assertEqual(calls[1][-1]['role'],'tool')
   self.assertEqual([x['role'] for x in state.history('s')],['user','assistant'])
 async def test_provider_fallback_and_unknown_tool(self):
  from infra.qqbot.service.model import Model
  from infra.qqbot.service.state import State
  with tempfile.TemporaryDirectory() as d:
   seen=[]
   async def transport(p,m,t):
    seen.append(p['model'])
    if p['model']=='bad':raise TimeoutError()
    return {'role':'assistant','content':'正常'}
   async def invoke(n,a):self.fail('unexpected tool invocation')
   model=Model({'providers':[{'model':'bad'},{'model':'ok'}]},State(Path(d)/'db'),invoke,transport)
   self.assertEqual(await model.answer('s','hi'),'正常')
   self.assertEqual(seen,['bad','ok'])

if __name__=='__main__':unittest.main()

