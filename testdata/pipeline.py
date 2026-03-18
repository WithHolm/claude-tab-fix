from __future__ import annotations

import json
import logging
from pathlib import Path
from typing import Any

logger = logging.getLogger(__name__)


class PipelineError(Exception):
    pass


class Stage:
    def __init__(self, name: str, config: dict[str, Any]) -> None:
        self.name = name
        self.config = config
        self._enabled = config.get("enabled", True)

    def run(self, data: list[dict]) -> list[dict]:
        if not self._enabled:
            logger.debug("stage %r skipped (disabled)", self.name)
            return data

        logger.info("running stage %r on %d items", self.name, len(data))
        results = []
        for item in data:
            try:
                out = self._process(item)
                if out is not None:
                    results.append(out)
            except Exception as exc:
                if self.config.get("fail_fast", False):
                    raise PipelineError(f"stage {self.name!r} failed on item {item!r}") from exc
                logger.warning("stage %r: skipping item due to error: %s", self.name, exc)
        return results

    def _process(self, item: dict) -> dict | None:
        raise NotImplementedError


class FilterStage(Stage):
    def _process(self, item: dict) -> dict | None:
        key = self.config["key"]
        values = self.config.get("values", [])
        if item.get(key) in values:
            return item
        return None


class TransformStage(Stage):
    def _process(self, item: dict) -> dict | None:
        mapping = self.config.get("mapping", {})
        return {mapping.get(k, k): v for k, v in item.items()}


class Pipeline:
    def __init__(self, stages: list[Stage]) -> None:
        self.stages = stages

    @classmethod
    def from_file(cls, path: Path) -> "Pipeline":
        config = json.loads(path.read_text())
        stages = []
        for stage_cfg in config.get("stages", []):
            kind = stage_cfg.pop("type")
            if kind == "filter":
                stages.append(FilterStage(stage_cfg.pop("name", kind), stage_cfg))
            elif kind == "transform":
                stages.append(TransformStage(stage_cfg.pop("name", kind), stage_cfg))
            else:
                raise PipelineError(f"unknown stage type: {kind!r}")
        return cls(stages)

    def run(self, data: list[dict]) -> list[dict]:
        result = data
        for stage in self.stages:
            result = stage.run(result)
            logger.debug("after stage %r: %d items", stage.name, len(result))
        return result
