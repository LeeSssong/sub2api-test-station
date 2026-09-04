import importlib.util
import inspect
import pathlib
import sys
import types
import unittest
from unittest import mock


ADAPTER_PATH = pathlib.Path(__file__).with_name("model-detector-v411-adapter.py")
SPEC = importlib.util.spec_from_file_location("model_detector_v411_adapter", ADAPTER_PATH)
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class V411AdapterTests(unittest.TestCase):
    def test_maps_complete_v411_report_to_bounded_sidecar_response(self):
        report = {
            "juice_verdict_state": "pass",
            "fingerprint_verdict_state": "strong_match",
            "fingerprint_model": "gpt-5.6-sol",
            "fingerprint_match": {"gpt-5.6-sol": 0.98, "gpt-5.6-terra": 0.12},
            "network_summary": {"logical_tasks": 49, "successful": 46},
            "fingerprint_summary": {"fingerprint_match": {"gpt-5.6-sol": 0.98, "gpt-5.6-terra": 0.12}},
        }

        response = MODULE.report_to_sidecar_response(report, "medium", "gpt-5.6-sol")

        self.assertEqual("normal", response["status"])
        self.assertEqual("complete", response["evidence_state"])
        self.assertEqual("pass", response["juice_status"])
        self.assertEqual("strong_match", response["fingerprint_status"])
        self.assertEqual("gpt-5.6-sol", response["fingerprint_candidate"])
        self.assertEqual(49, response["planned_requests"])
        self.assertEqual(46, response["valid_samples"])
        self.assertEqual("4.1.1", response["detector_version"])
        self.assertNotIn("api_key", str(response))

    def test_maps_incomplete_report_to_explicit_failure_without_inventing_fingerprint(self):
        report = {
            "juice_verdict_state": "insufficient",
            "fingerprint_verdict_state": "unclear",
            "network_summary": {"logical_tasks": 19, "successful": 3},
            "fingerprint_summary": {"fingerprint_match": {"gpt-5.6-sol": 0.44}},
        }

        response = MODULE.report_to_sidecar_response(report, "low", "gpt-5.6-sol")

        self.assertEqual("failed", response["status"])
        self.assertEqual("insufficient", response["evidence_state"])
        self.assertEqual("unclear", response["fingerprint_status"])
        self.assertEqual("", response["fingerprint_candidate"])
        self.assertEqual(19, response["planned_requests"])
        self.assertEqual(3, response["valid_samples"])

    def test_keeps_unclear_supported_candidate_when_evidence_is_complete(self):
        report = {
            "juice_verdict_state": "pass",
            "fingerprint_verdict_state": "unclear",
            "fingerprint_model": "gpt-5.6-luna",
            "fingerprint_match": {"gpt-5.6-luna": 0.987, "gpt-5.6-sol": 0.01},
            "network_summary": {"logical_tasks": 49, "successful": 48},
        }

        response = MODULE.report_to_sidecar_response(report, "medium", "gpt-5.6-luna")

        self.assertEqual("normal", response["status"])
        self.assertEqual("complete", response["evidence_state"])
        self.assertEqual("gpt-5.6-luna", response["fingerprint_candidate"])
        self.assertEqual(0.987, response["fingerprint_similarity"]["gpt-5.6-luna"])

    def test_uses_juice_pass_as_an_explicit_normal_result_when_fingerprint_is_unclear(self):
        response = MODULE.report_to_sidecar_response({
            "juice_verdict_state": "pass",
            "fingerprint_verdict_state": "unclear",
            "network_summary": {"logical_tasks": 49, "successful": 48},
        }, "medium", "gpt-5.6-sol")
        self.assertEqual("normal", response["status"])

    def test_maps_unavailable_detector_execution_to_failed(self):
        self.assertEqual("failed", MODULE.unavailable_response("medium", "detector_error")["status"])

    def test_maps_possible_non_gpt_to_abnormal(self):
        response = MODULE.report_to_sidecar_response({
            "juice_verdict_state": "possible_non_gpt",
            "fingerprint_verdict_state": "unclear",
            "network_summary": {"logical_tasks": 19, "successful": 19},
        }, "low", "gpt-5.6-sol")
        self.assertEqual("abnormal", response["status"])

    def test_rejects_unauthorized_requests(self):
        self.assertFalse(MODULE.authorized("wrong", "expected"))
        self.assertTrue(MODULE.authorized("expected", "expected"))

    def test_preserves_official_preset_workers_for_formal_fingerprint_policy(self):
        source = inspect.getsource(MODULE.run_v411)
        self.assertNotIn('config["workers"] = 1', source)
        self.assertNotIn('config["workers"] =', source)

        captured = {}
        detector_module = types.ModuleType("gpt56_vnext.detector")
        presets_module = types.ModuleType("gpt56_vnext.presets")
        package = types.ModuleType("gpt56_vnext")

        class Session:
            def __init__(self, **kwargs):
                captured.update(kwargs["config"])

            def run_single(self):
                return {"juice_verdict_state": "pass", "network_summary": {"logical_tasks": 19, "successful": 19}}

            def close(self):
                pass

        detector_module.DetectorSession = Session
        presets_module.get_preset = lambda _mode, _profile: {"workers": 8, "official": True, "config_hash": "official-hash"}
        with mock.patch.dict(sys.modules, {"gpt56_vnext": package, "gpt56_vnext.detector": detector_module, "gpt56_vnext.presets": presets_module}):
            MODULE.run_v411({"profile": "low", "declared_model": "gpt-5.6-sol", "request_model": "gpt-5.6-sol", "base_url": "https://example.test", "api_key": "secret"})

        self.assertEqual(8, captured["workers"])
        self.assertTrue(captured["official"])
        self.assertEqual("official-hash", captured["config_hash"])

    def test_maps_juice_mismatch_to_bounded_explainable_evidence(self):
        response = MODULE.report_to_sidecar_response({
            "juice_verdict_state": "mismatch",
            "network_summary": {"logical_tasks": 19, "successful": 19},
            "juice_summary": {
                "claimed_model": "gpt-5.6-sol",
                "mixed_models_observed": ["gpt-5.6-luna"],
                "sticky_events": [{"evidence": {
                    "effort": "high",
                    "normalized_value": "12345",
                    "mixed_models": ["gpt-5.6-luna"],
                    "output": "must not survive",
                }}],
            },
        }, "low", "gpt-5.6-sol")

        self.assertEqual("gpt-5.6-sol", response["juice_summary"]["claimed_model"])
        self.assertEqual(["gpt-5.6-luna"], response["juice_summary"]["conflicting_models"])
        self.assertEqual([{
            "effort": "high", "observed_value": "12345",
            "matching_models": ["gpt-5.6-luna"],
        }], response["juice_summary"]["mismatch_samples"])
        self.assertNotIn("output", str(response["juice_summary"]))


if __name__ == "__main__":
    unittest.main()
