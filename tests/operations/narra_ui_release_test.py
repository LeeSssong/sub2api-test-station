import unittest,importlib.util,pathlib
p=pathlib.Path(__file__).resolve().parents[2]/'ops/deploy-narra-ui-blue-green.py';s=importlib.util.spec_from_file_location('ui_release',p);m=importlib.util.module_from_spec(s);s.loader.exec_module(m)
class BlueGreen(unittest.TestCase):
 def test_web_replacement_preserves_worker_and_running_app(self):
  original={'services':{'narra-image':{'image':'old','environment':{'APP_URL':'http://test'},'depends_on':['narra-worker']},'narra-worker':{'image':'old-worker','command':['worker']},'narra-cleanup':{'image':'old-cleaner'}}}
  result,name=m.app_config(original,'narra-image','new')
  self.assertEqual(result['services']['narra-worker'],original['services']['narra-worker'])
  self.assertEqual(result['services']['narra-image']['image'],'old')
  self.assertEqual(result['services'][name]['image'],'new')
  self.assertNotIn('depends_on',result['services'][name]);self.assertNotIn(name,original['services'])
if __name__=='__main__':unittest.main()
