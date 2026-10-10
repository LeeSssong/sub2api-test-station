import importlib.util,unittest,pathlib
p=pathlib.Path(__file__).resolve().parents[2]/'ops/deploy-narra-test-station.py'
spec=importlib.util.spec_from_file_location('narra_deploy',p);m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
class TargetTests(unittest.TestCase):
 def test_test_station_identity_is_fixed(self):
  self.assertEqual(str(m.ROOT),'/opt/sub2api-test-station')
  self.assertEqual(m.PROJECT,'sub2api-test-station')
 def test_loads_existing_release_controller(self):
  base=m.load_base(p.parent/'deploy-sub2api-test-station-api.py')
  c=':80 {\n  reverse_proxy test-station-api-green:8080\n}\n'
  self.assertIn('test-station-api-blue:8080',base.change_upstream(c,'test-station-api-green','test-station-api-blue'))
class RestoreProofTests(unittest.TestCase):
 def test_accepts_only_matching_complete_restore(self):
  manifest={'binary_sha256':'binary','migration_set_sha256':'migrations'}
  proof={**manifest,'result':'passed','backup_sha256':'backup','restored_table_count':150}
  m.validate_restore_proof(proof,manifest,'backup')
  for patch in ({'result':'failed'},{'binary_sha256':'other'},{'migration_set_sha256':'other'},{'backup_sha256':'other'},{'restored_table_count':0}):
   with self.assertRaises(RuntimeError):m.validate_restore_proof({**proof,**patch},manifest,'backup')
if __name__=='__main__':unittest.main()
