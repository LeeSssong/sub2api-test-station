import sys, unittest, tempfile
from pathlib import Path
sys.path.insert(0, str(Path(__file__).resolve().parents[3]))

class NativeServiceTests(unittest.TestCase):
    def test_service_has_no_astrbot_dependency(self):
        from infra.qqbot.service.business import Business
        self.assertTrue(callable(Business.xingqiao_public_config))

    def test_group_gate_requires_allowed_group_and_real_mention(self):
        from infra.qqbot.service.protocol import route_event
        cfg = {'allowed_groups':['12345'], 'admins':['7'], 'wake_prefix':['@小星'], 'allow_private':False}
        event = {'post_type':'message','message_type':'group','self_id':9,'user_id':8,'group_id':12345,'message_id':1,'message':[{'type':'text','data':{'text':'你好'}}]}
        self.assertIsNone(route_event(event,cfg))
        event['message'].insert(0, {'type':'at','data':{'qq':'9'}})
        self.assertEqual(route_event(event,cfg)['session'],'napcat:GroupMessage:12345')
        event['group_id']=54321
        self.assertIsNone(route_event(event,cfg))
        event['user_id']=9
        self.assertIsNone(route_event(event,cfg))

    def test_private_gate_and_self_message(self):
        from infra.qqbot.service.protocol import route_event
        cfg={'allowed_groups':[], 'admins':['7'],'wake_prefix':[], 'allow_private':False}
        e={'post_type':'message','message_type':'private','self_id':9,'user_id':8,'message_id':1,'message':[{'type':'text','data':{'text':'hi'}}]}
        self.assertIsNone(route_event(e,cfg))
        e['user_id']=7
        self.assertEqual(route_event(e,cfg)['target'],7)
        e['post_type']='message_sent'
        self.assertIsNone(route_event(e,cfg))

    def test_event_dedup_persists_across_restart(self):
        from infra.qqbot.service.state import State
        with tempfile.TemporaryDirectory() as d:
            s=State(Path(d)/'state.sqlite3')
            self.assertTrue(s.claim('9:group:123:1'))
            self.assertFalse(s.claim('9:group:123:1'))
            self.assertFalse(State(Path(d)/'state.sqlite3').claim('9:group:123:1'))
            s.save_history('a',[{'role':'user','content':'hello'}])
            self.assertEqual(s.history('a')[0]['content'],'hello')

class TransportTests(unittest.IsolatedAsyncioTestCase):
    async def test_action_correlation_and_rejection(self):
        from infra.qqbot.service.protocol import OneBot, ActionRejected
        import asyncio
        bot=OneBot('ws://localhost','test')
        class Wire:
            async def send_json(self,payload):
                self.payload=payload
        bot.ws=Wire()
        task=asyncio.create_task(bot.call('get_status',{}))
        await asyncio.sleep(0)
        bot.receive({'echo':bot.ws.payload['echo'],'status':'ok','retcode':0,'data':{'online':True}})
        self.assertEqual(await task,{'online':True})
        task=asyncio.create_task(bot.call('send_group_msg',{}))
        await asyncio.sleep(0)
        bot.receive({'echo':bot.ws.payload['echo'],'status':'failed','retcode':100})
        with self.assertRaises(ActionRejected): await task

    async def test_disconnect_fails_pending_without_resend(self):
        from infra.qqbot.service.protocol import OneBot
        import asyncio
        bot=OneBot('ws://localhost','test')
        class Wire:
            async def send_json(self,p): pass
        bot.ws=Wire()
        task=asyncio.create_task(bot.call('send_group_msg',{}))
        await asyncio.sleep(0)
        bot.disconnected()
        with self.assertRaises(ConnectionError): await task
        self.assertFalse(bot.pending)

if __name__=='__main__': unittest.main()


class EventAccountTests(unittest.TestCase):
 def test_foreign_account_events_are_rejected(self):
  from infra.qqbot.service.protocol import route_event
  cfg={'self_id':'9','allowed_groups':['12345'],'admins':[],'wake_prefix':['/']}
  e={'post_type':'message','message_type':'group','self_id':10,'user_id':8,'group_id':12345,'message_id':1,'message':[{'type':'text','data':{'text':'/help'}}]}
  self.assertIsNone(route_event(e,cfg))
