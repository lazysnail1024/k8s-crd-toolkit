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


def exact_stage_inventory(observed, contract, platform):
    expected = contract['stages']
    if set(observed) != set(expected):
        return False
    for stage, names in expected.items():
        actual = observed[stage]
        allowed = [set(names)]
        alternative = contract.get('platformAlternatives', {}).get(platform, {}).get(stage)
        if alternative:
            allowed.append(set(names) - set(alternative['remove']) | set(alternative['add']))
        if set(actual) not in allowed or not all(type(v) is bool and v for v in actual.values()):
            return False
    return True
