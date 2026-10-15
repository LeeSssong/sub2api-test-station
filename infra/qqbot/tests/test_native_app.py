import unittest,tempfile,sys,asyncio
from pathlib import Path
sys.path.insert(0,str(Path(__file__).resolve().parents[3]))

class AppTests(unittest.IsolatedAsyncioTestCase):
 async def test_duplicate_message_only_calls_model_once(self):
  from infra.qqbot.service.app import App
  with tempfile.TemporaryDirectory() as d:
   cfg={'onebot_url':'ws://localhost','onebot_token':'test','allowed_groups':['12345'],'admins':[],'wake_prefix':[],'providers':[]}
   app=App(cfg,d);sent=[];asked=[]
   async def answer(s,t):asked.append(t);return '**你好**'
   async def send(k,t,m):sent.append(m);return {'message_id':2}
   app.model.answer=answer;app.send=send
   event={'post_type':'message','message_type':'group','self_id':9,'user_id':8,'group_id':12345,'message_id':1,'message':[{'type':'at','data':{'qq':'9'}},{'type':'text','data':{'text':'你好'}}]}
   await app.handle(event);await app.handle(event)
   self.assertEqual(asked,['你好']);self.assertEqual(sent[0][0]['data']['text'],'你好')
 async def test_unauthorized_reset_does_not_change_history(self):
  from infra.qqbot.service.app import App
  with tempfile.TemporaryDirectory() as d:
   app=App({'onebot_url':'ws://localhost','onebot_token':'t','allowed_groups':['12345'],'admins':[],'wake_prefix':[],'providers':[]},d)
   app.state.save_history('napcat:GroupMessage:12345',[{'role':'user','content':'preserve'}])
   event={'post_type':'message','message_type':'group','self_id':9,'user_id':8,'group_id':12345,'message_id':1,'message':[{'type':'at','data':{'qq':'9'}},{'type':'text','data':{'text':'reset'}}]}
   await app.handle(event)
   self.assertEqual(app.state.history('napcat:GroupMessage:12345')[0]['content'],'preserve')

