import unittest

from c4.utils.slimmer import ContextSlimmer


class TestContextSlimmer(unittest.TestCase):
    def test_slim_log(self):
        log = "line1\nline2\nERROR: something failed\nline4\n" + "normal\n" * 200 + "end"
        slimmed = ContextSlimmer.slim_log(log, max_lines=50)
        self.assertIn("ERROR: something failed", slimmed)
        self.assertIn("truncated", slimmed)
        self.assertLess(len(slimmed.splitlines()), 100)

    def test_slim_json(self):
        data = {"items": list(range(100)), "meta": "data"}
        slimmed_str = ContextSlimmer.slim_json(data, max_list_len=5)
        self.assertIn("... (95 more items) ...", slimmed_str)
        self.assertIn('"meta": "data"', slimmed_str)

    def test_slim_code(self):
        code = "import os\nclass MyClass:\n    def method(self):\n        pass\n\ndef my_func():\n    return 1"
        slimmed = ContextSlimmer.slim_code(code)
        self.assertIn("class MyClass:", slimmed)
        self.assertIn("def method(self):", slimmed)
        self.assertIn("def my_func():", slimmed)
        self.assertIn("omitted", slimmed)

if __name__ == "__main__":
    unittest.main()

