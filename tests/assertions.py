"""The JSON protocol emitted by the independent Go consumer."""
import json


def parse_assertions(stdout):
    def unique(pairs):
        result = {}
        for name, value in pairs:
            if name in result:
                raise ValueError('duplicate assertion: ' + name)
            result[name] = value
        return result

    values = json.loads(stdout, object_pairs_hook=unique)
    if not isinstance(values, dict) or not values:
        raise ValueError('consumer must emit a nonempty assertion object')
    if any(not name or type(value) is not bool for name, value in values.items()):
        raise ValueError('assertions require nonempty names and JSON booleans')
    return values
