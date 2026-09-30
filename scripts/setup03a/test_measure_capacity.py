"""Protect the probe against reporting success after failed cleanup or shutdown."""
import importlib.util
import pathlib
import sys
import unittest

sys.dont_write_bytecode = True

spec = importlib.util.spec_from_file_location("capacity", pathlib.Path(__file__).with_name("measure-capacity.py"))
capacity = importlib.util.module_from_spec(spec)
spec.loader.exec_module(capacity)


class CompletionTest(unittest.TestCase):
    def test_only_complete_clean_delivery_passes(self):
        self.assertTrue(capacity.transport_succeeded("all_offered_work_accounted", None, 0, 0, [{}]))
        for reason, failure, application_exit, cleanup_exit, sources in [
            ("all_offered_work_accounted", None, 1, 0, [{}]),
            ("all_offered_work_accounted", None, 0, 1, [{}]),
            ("all_offered_work_accounted", "log capture failed", 0, 0, [{}]),
            ("all_offered_work_accounted", None, None, 0, [{}]),
            ("all_offered_work_accounted", None, 0, 0, []),
            ("all_offered_work_accounted", None, 0, 0, [{"lost_total": 1}]),
            ("all_offered_work_accounted", None, 0, 0, [{"error_count": 1}]),
            ("all_offered_work_accounted", None, 0, 0, [{"seed_lost": 1}]),
            ("forced_application_stop", None, 0, 0, [{}]),
        ]:
            with self.subTest(reason=reason, failure=failure, application_exit=application_exit,
                              cleanup_exit=cleanup_exit, sources=sources):
                self.assertFalse(capacity.transport_succeeded(reason, failure, application_exit, cleanup_exit, sources))


if __name__ == "__main__":
    unittest.main()
