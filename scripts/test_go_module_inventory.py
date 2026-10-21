"""Regression checks for release graph integrity and inventory privacy."""
import json
import unittest
from generate_go_module_inventory import inventory_from_stream


class ModuleInventoryTests(unittest.TestCase):
    def stream(self, *modules):
        return '\n'.join(json.dumps(module) for module in modules)

    def test_versioned_replacement_preserves_effective_input_and_omits_paths(self):
        raw = self.stream(
            {'Path': 'example.org/app', 'Main': True, 'Dir': '/private/checkout', 'GoMod': '/private/go.mod'},
            {'Path': 'example.org/dependency', 'Version': 'v1.0.0', 'Indirect': True,
             'Sum': 'h1:original', 'Dir': '/private/cache',
             'Replace': {'Path': 'example.org/fork', 'Version': 'v1.2.0', 'Sum': 'h1:replacement',
                         'GoModSum': 'h1:mod', 'Dir': '/private/fork', 'GoMod': '/private/fork/go.mod'}})
        result = inventory_from_stream(raw)
        dependency = next(module for module in result['modules'] if not module.get('Main'))
        self.assertEqual(dependency['Replace']['Version'], 'v1.2.0')
        self.assertEqual(dependency['Replace']['Sum'], 'h1:replacement')
        self.assertEqual(dependency['Version'], 'v1.0.0')
        self.assertTrue(dependency['Indirect'])
        self.assertNotIn('/private', json.dumps(result))

    def test_order_is_independent_of_go_output_order(self):
        main = {'Path': 'example.org/z', 'Main': True}
        dependency = {'Path': 'example.org/a', 'Version': 'v1.0.0'}
        self.assertEqual(inventory_from_stream(self.stream(main, dependency)),
                         inventory_from_stream(self.stream(dependency, main)))

    def test_local_replacement_is_rejected(self):
        with self.assertRaisesRegex(ValueError, 'versioned'):
            inventory_from_stream(self.stream({'Path': 'example.org/app', 'Main': True,
                                               'Replace': {'Path': '/private/source'}}))

    def test_incomplete_or_corrupt_graph_is_rejected(self):
        invalid = ['', '{}', '[]', '{', self.stream({'Path': 'example.org/dep'}),
                   self.stream({'Path': 'example.org/app', 'Main': True}, {'Path': 'example.org/app'}),
                   self.stream({'Path': 'example.org/app', 'Main': True, 'Error': {'Err': 'missing module'}})]
        for stream in invalid:
            with self.subTest(stream=stream), self.assertRaises(ValueError):
                inventory_from_stream(stream)


if __name__ == '__main__':
    unittest.main()
