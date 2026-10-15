import unittest,tempfile,json,sqlite3,sys
from pathlib import Path
sys.path.insert(0,str(Path(__file__).resolve().parents[3]))
from infra.qqbot.service.migrate import migrate
from infra.qqbot.service.state import State

class MigrationTests(unittest.TestCase):
 def test_import_preserves_selected_history_and_secrets_without_overwrite(self):
  with tempfile.TemporaryDirectory() as d:
   src=Path(d)/'old';(src/'data').mkdir(parents=True);(src/'kb').mkdir()
   (src/'kb/a.md').write_text('knowledge')
   cfg={'agent_runner':{'config':{'model':{'provider_id':'p'},'persona':{'persona_id':'x'}}},'provider':[{'id':'p','enable':True,'provider_source_id':'s','model':'m'}],'provider_sources':[{'id':'s','enable':True,'key':['secret'],'api_base':'https://example.test/v1'}],'platform':[{'id':'napcat','enable':True,'type':'aiocqhttp'}],'platform_settings':{'enable_id_white_list':True,'id_whitelist':['12345']},'admins_id':['7'],'wake_prefix':['/']}
   (src/'data/cmd_config.json').write_text(json.dumps(cfg))
   with sqlite3.connect(src/'data/data_v4.db') as db:
    db.execute('create table personas (persona_id text,system_prompt text)');db.execute("insert into personas values ('x','persona')")
    db.execute('create table conversations (user_id text,content text,platform_id text,conversation_id text)')
    for ident,text in [('selected','keep'),('other','wrong')]:db.execute('insert into conversations values (?,?,?,?)',('napcat:GroupMessage:12345',json.dumps([{'role':'user','content':text}]),'napcat',ident))
    db.execute('create table preferences (scope text,scope_id text,key text,value text)');db.execute('insert into preferences values (?,?,?,?)',('umo','napcat:GroupMessage:12345','sel_conv_id',json.dumps({'val':'selected'})))
   dest=Path(d)/'new';migrate(src,dest)
   self.assertEqual(State(dest/'data/state.sqlite3').history('napcat:GroupMessage:12345')[0]['content'],'keep')
   self.assertEqual(json.loads((dest/'config.json').read_text())['providers'][0]['key'],'secret')
   self.assertEqual((dest/'config.json').stat().st_mode&0o777,0o600)
   with self.assertRaises(RuntimeError):migrate(src,dest)


class ConversationSelectionTests(unittest.TestCase):
 def test_ambiguous_history_without_selection_is_rejected(self):
  from infra.qqbot.service.migrate import select_conversation
  with self.assertRaises(ValueError):select_conversation([('old','[]'),('new','[]')],None)
 def test_selected_history_wins(self):
  from infra.qqbot.service.migrate import select_conversation
  self.assertEqual(select_conversation([('old','a'),('new','b')],'new'),'b')
